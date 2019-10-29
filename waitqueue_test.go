package waitqueue

import (
	"fmt"
	"testing"
	"time"
)

func TestWaitqueue(t *testing.T) {
	testWQ(1)
}

type wqTestData struct {
	beginTime time.Time
	timeout   time.Duration
	index     int
	describe  string
	wi        *WaitItem
}

func testHandler(data interface{}) {
	d := data.(*wqTestData)
	fmt.Printf("%s:%d,timeout[%d],beginTime[%v],endTime[%v].\r\n", d.describe, d.index, d.timeout, d.beginTime, time.Now())
}

func testWQ(num int) {
	wq := NewWaitQueue(num)

	d1 := &wqTestData{
		beginTime: time.Now(),
		timeout:   1 * time.Second,
		index:     1,
		describe:  "test",
	}
	wi1, _ := wq.Add(d1, d1.timeout, testHandler)
	d1.wi = wi1

	d2 := &wqTestData{
		beginTime: time.Now(),
		timeout:   3 * time.Second,
		index:     2,
		describe:  "test",
	}
	wi2, _ := wq.Add(d2, d2.timeout, testHandler)
	d2.wi = wi2

	d3 := &wqTestData{
		beginTime: time.Now(),
		timeout:   5 * time.Second,
		index:     3,
		describe:  "test",
	}
	wi3, _ := wq.Add(d3, d3.timeout, testHandler)
	d3.wi = wi3

	d4 := &wqTestData{
		beginTime: time.Now(),
		timeout:   10 * time.Second,
		index:     4,
		describe:  "test",
	}
	wi4, _ := wq.Add(d4, d4.timeout, testHandler)
	d4.wi = wi4

	time.Sleep(2 * time.Second)

	wi2.Remove()

	time.Sleep(20 * time.Second)
	wq.Destroy()
}
