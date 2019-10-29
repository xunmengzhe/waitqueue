package waitqueue

import (
	"container/list"
	"fmt"
	"github.com/pools"
	"sync"
	"time"
)

type TimeoutHandler func(data interface{})

type WaitItem struct {
	using   bool
	wq      *WaitQueue
	litem   *list.Element
	timeout time.Time
	data    interface{}
	handler TimeoutHandler
}

func (wi *WaitItem) Data() interface{} {
	if !wi.using {
		return nil
	} else {
		return wi.data
	}
}

func (wi *WaitItem) Remove() interface{} {
	if !wi.using {
		return nil
	}
	data, _ := wi.wq.remove(wi)
	return data
}

func (wi *WaitItem) Reset(timeout time.Duration) error {
	if !wi.using {
		return fmt.Errorf("Invalid WaitItem object.")
	}
	return wi.wq.resetItem(wi, timeout)
}

type WaitQueue struct {
	inited     bool
	destroying bool
	signalChan chan int
	usingCnt   int
	ltask      *list.List
	lock       sync.Mutex
	timer      *time.Timer
	gopool     *pools.GoPool
}

const defaultTimeout time.Duration = 5 * time.Second //10 * time.Minute

func NewWaitQueue(concurrentNumber int) *WaitQueue {
	timer := time.NewTimer(defaultTimeout)
	lt := list.New()
	gopool := pools.NewGoPool(concurrentNumber)
	wq := &WaitQueue{
		inited:     true,
		destroying: false,
		signalChan: make(chan int, 1),
		usingCnt:   0,
		ltask:      lt,
		timer:      timer,
		gopool:     gopool,
	}
	go wq.worker()
	return wq
}

func (w *WaitQueue) worker() {
	for {
		select {
		case <-w.timer.C:
			w.lock.Lock()
			for {
				if w.ltask.Len() <= 0 {
					w.timer.Reset(defaultTimeout)
					break
				} else {
					lit := w.ltask.Front()
					wit := lit.Value.(*WaitItem)
					if !time.Now().Before(wit.timeout) { //timeout
						w.ltask.Remove(lit)
						wit.using = false
						w.gopool.AddWorker(wit.data, wit.handler)
						continue
					} else {
						duration := wit.timeout.Sub(time.Now())
						if duration < 0 {
							duration = 0
						}
						w.timer.Reset(duration)
						break
					}
				}
			}
			w.lock.Unlock()
		case <-w.signalChan:
			w.lock.Lock()
			if w.destroying { // destorying
				w.lock.Unlock()
				return
			}
			w.lock.Unlock()
		}
	}
}

func (w *WaitQueue) destroy() {
	if w.inited && w.destroying && w.usingCnt <= 0 {
		w.inited = false
		w.signalChan <- 1
		w.timer.Stop()
		w.gopool.Destroy()
		w.ltask.Init() //清空链表
	}
}

func (w *WaitQueue) get() bool {
	if w.inited && !w.destroying {
		w.usingCnt++
		return true
	}
	return false
}

func (w *WaitQueue) put() {
	if w.inited {
		w.usingCnt--
		if w.destroying && w.usingCnt == 0 {
			w.destroy()
		}
	}
}

func (w *WaitQueue) Destroy() {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.inited {
		w.destroying = true
		if w.usingCnt > 0 {
			return
		}
		w.destroy()
	}
}

//超时触发handler操作
func (w *WaitQueue) Add(data interface{}, timeout time.Duration, handler TimeoutHandler) (*WaitItem, error) {
	if data == nil {
		return nil, fmt.Errorf("data is null.")
	}
	if handler == nil {
		return nil, fmt.Errorf("handler is null.")
	}
	if timeout < 0 {
		timeout = 0
	}

	w.lock.Lock()
	defer w.lock.Unlock()
	if !w.get() {
		return nil, fmt.Errorf("Invalid wait queue.")
	}
	defer w.put()

	wit := &WaitItem{
		using:   true,
		wq:      w,
		data:    data,
		handler: handler,
		timeout: time.Now().Add(timeout),
	}
	if w.ltask.Len() == 0 {
		lit2 := w.ltask.PushBack(wit)
		wit.litem = lit2
		w.timer.Reset(timeout)
	} else {
		leit := w.ltask.Back()
		lbit := w.ltask.Front()
		lit := lbit
		for {
			wit2 := lit.Value.(*WaitItem)
			if wit.timeout.Before(wit2.timeout) {
				lit2 := w.ltask.InsertBefore(wit, lit)
				wit.litem = lit2
				break
			}
			if lit == leit {
				lit2 := w.ltask.PushBack(wit)
				wit.litem = lit2
				break
			}
			lit = lit.Next()
		}
		if w.ltask.Front() != lbit { //新的waitItem插入到了队列头部，所以要重置定时器
			duration := wit.timeout.Sub(time.Now())
			if duration < 0 {
				duration = 0
			}
			w.timer.Reset(duration) //此处一定要重新计算时间，因为在循环期间时间在流逝
		}
	}
	return wit, nil
}

func (w *WaitQueue) remove(item *WaitItem) (interface{}, error) {
	if item == nil {
		return nil, fmt.Errorf("Invalid WaitItem object.")
	}

	w.lock.Lock()
	defer w.lock.Unlock()
	if !w.get() {
		return nil, fmt.Errorf("Invalid wait queue.")
	}
	defer w.put()
	lbit := w.ltask.Front()
	w.ltask.Remove(item.litem)
	if lbit == item.litem {
		if w.ltask.Len() > 0 {
			lbit = w.ltask.Front()
			wit := lbit.Value.(*WaitItem)
			duration := wit.timeout.Sub(time.Now())
			if duration < 0 {
				duration = 0
			}
			w.timer.Reset(duration)
		} else {
			w.timer.Reset(defaultTimeout)
		}
	}
	data := item.data
	item.using = false
	item.handler = nil
	item.data = nil
	item.litem = nil
	item.wq = nil
	return data, nil
}

func (w *WaitQueue) resetItem(item *WaitItem, timeout time.Duration) error {
	if item == nil {
		return fmt.Errorf("Invalid WaitItem object.")
	}

	w.lock.Lock()
	defer w.lock.Unlock()
	if !w.get() {
		return fmt.Errorf("Invalid wait queue.")
	}
	defer w.put()
	if w.ltask.Len() <= 0 {
		return fmt.Errorf("Invalid WaitItem object.")
	}
	if w.ltask.Len() == 1 { //只有一个item
		item.timeout = time.Now().Add(timeout)
		w.timer.Reset(timeout)
	} else {
		lbit := w.ltask.Front() //一定要在remove前获取lbit
		w.ltask.Remove(item.litem)
		leit := w.ltask.Back() //一定要在remove后获取leit
		lit := w.ltask.Front()
		for {
			item2 := lit.Value.(*WaitItem)
			if item.timeout.Before(item2.timeout) {
				lit2 := w.ltask.InsertBefore(item, lit)
				item.litem = lit2
				break
			}
			if lit == leit {
				lit2 := w.ltask.PushBack(item)
				item.litem = lit2
				break
			}
			lit = lit.Next()
		}
		if w.ltask.Front() != lbit { //队列头部已经变化，所以要重置定时器
			duration := item.timeout.Sub(time.Now())
			if duration < 0 {
				duration = 0
			}
			w.timer.Reset(duration) //此处一定要重新计算时间，因为在循环期间时间在流逝
		}
	}
	return nil
}
