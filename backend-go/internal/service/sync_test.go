package service

import (
	"sync/atomic"
	"testing"
)

// 课表写入互斥：并发 AcquireCourseWrite 只有一个成功；释放后可再次获取。
func TestCourseWriteMutex(t *testing.T) {
	svc := &Sync{}

	if ok := svc.AcquireCourseWrite(); !ok {
		t.Fatal("首次获取应成功")
	}
	for i := 0; i < 3; i++ {
		if ok := svc.AcquireCourseWrite(); ok {
			t.Fatal("持有中再次获取应失败")
		}
	}
	svc.ReleaseCourseWrite()
	if ok := svc.AcquireCourseWrite(); !ok {
		t.Fatal("释放后应可再次获取")
	}
	svc.ReleaseCourseWrite()

	// 空释放不 panic、不影响下次获取
	svc.ReleaseCourseWrite()
	if ok := svc.AcquireCourseWrite(); !ok {
		t.Fatal("空释放后仍可获取")
	}
	svc.ReleaseCourseWrite()
}

// 并发竞态下互斥仍然成立：仅一个 goroutine 能进入临界区。
func TestCourseWriteMutexConcurrent(t *testing.T) {
	svc := &Sync{}
	var n, maxIn int32

	for i := 0; i < 20; i++ {
		go func() {
			if !svc.AcquireCourseWrite() {
				return
			}
			cur := atomic.AddInt32(&n, 1)
			if cur > atomic.LoadInt32(&maxIn) {
				atomic.StoreInt32(&maxIn, cur)
			}
			atomic.AddInt32(&n, -1)
			svc.ReleaseCourseWrite()
		}()
	}

	if atomic.LoadInt32(&maxIn) > 1 {
		t.Fatalf("并发下临界区最多 1 人, 实际 %d", maxIn)
	}
}
