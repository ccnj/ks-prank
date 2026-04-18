package worker

import (
	"log"
	"sync"
)

// Task 单个待执行任务
type Task struct {
	Name        string
	WorkerGroup int
	Run         func()
}

// Dispatcher 按 worker_group 路由任务：同 group 串行、跨 group 并行
type Dispatcher struct {
	queues map[int]chan Task
	mu     sync.Mutex

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
	qSize    int
}

func NewDispatcher(queueSize int) *Dispatcher {
	if queueSize <= 0 {
		queueSize = 100
	}
	return &Dispatcher{
		queues: make(map[int]chan Task),
		stopCh: make(chan struct{}),
		qSize:  queueSize,
	}
}

func (d *Dispatcher) ensureQueue(group int) chan Task {
	d.mu.Lock()
	defer d.mu.Unlock()
	if q, ok := d.queues[group]; ok {
		return q
	}
	q := make(chan Task, d.qSize)
	d.queues[group] = q
	d.wg.Add(1)
	go d.runWorker(group, q)
	return q
}

func (d *Dispatcher) Dispatch(t Task) {
	q := d.ensureQueue(t.WorkerGroup)
	select {
	case q <- t:
	case <-d.stopCh:
	}
}

func (d *Dispatcher) Stop() {
	d.stopOnce.Do(func() {
		close(d.stopCh)
		d.mu.Lock()
		for _, q := range d.queues {
			close(q)
		}
		d.mu.Unlock()
		d.wg.Wait()
	})
}

func (d *Dispatcher) runWorker(group int, q <-chan Task) {
	defer d.wg.Done()
	for {
		select {
		case <-d.stopCh:
			return
		case t, ok := <-q:
			if !ok {
				return
			}
			log.Printf("worker-g%d 开始: %s", group, t.Name)
			t.Run()
			log.Printf("worker-g%d 完成: %s", group, t.Name)
		}
	}
}
