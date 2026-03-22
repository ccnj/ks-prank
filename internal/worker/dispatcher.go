package worker

import (
	"log"
	"sync"
)

type GiftTask struct {
	GiftName   string
	Count      int
	KsNickname string
	KsAvatar   string
}

type GiftAction func(task GiftTask)

type GiftDispatcher struct {
	actions       map[string]GiftAction
	workerQueues  []chan GiftTask
	giftToWorker  map[string]int
	defaultWorker chan GiftTask

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewGiftDispatcher(actions map[string]GiftAction, giftGroups [][]string, queueSize int) *GiftDispatcher {
	if queueSize <= 0 {
		queueSize = 100
	}

	d := &GiftDispatcher{
		actions:       actions,
		workerQueues:  make([]chan GiftTask, 0, len(giftGroups)),
		giftToWorker:  make(map[string]int),
		defaultWorker: make(chan GiftTask, queueSize),
		stopCh:        make(chan struct{}),
	}

	for i, group := range giftGroups {
		q := make(chan GiftTask, queueSize)
		d.workerQueues = append(d.workerQueues, q)
		for _, gift := range group {
			if oldIdx, exists := d.giftToWorker[gift]; exists {
				log.Printf("礼物 %s 被重复分组，沿用 worker-%d，忽略 worker-%d", gift, oldIdx+1, i+1)
				continue
			}
			d.giftToWorker[gift] = i
		}
	}

	return d
}

func (d *GiftDispatcher) Start() {
	for i, q := range d.workerQueues {
		d.wg.Add(1)
		go d.runWorker(i+1, q)
	}

	d.wg.Add(1)
	go d.runWorker(0, d.defaultWorker)
}

func (d *GiftDispatcher) Stop() {
	d.stopOnce.Do(func() {
		close(d.stopCh)
		close(d.defaultWorker)
		for _, q := range d.workerQueues {
			close(q)
		}
		d.wg.Wait()
	})
}

func (d *GiftDispatcher) Dispatch(task GiftTask) {
	if _, ok := d.actions[task.GiftName]; !ok {
		return
	}

	if idx, ok := d.giftToWorker[task.GiftName]; ok {
		d.workerQueues[idx] <- task
		return
	}

	d.defaultWorker <- task
}

func (d *GiftDispatcher) runWorker(workerID int, q <-chan GiftTask) {
	defer d.wg.Done()

	for {
		select {
		case <-d.stopCh:
			return
		case task, ok := <-q:
			if !ok {
				return
			}
			action, exists := d.actions[task.GiftName]
			if !exists {
				continue
			}
			log.Printf("worker-%d 开始执行: %s x%d (来自 %s)", workerID, task.GiftName, task.Count, task.KsNickname)
			action(task)
			log.Printf("worker-%d 执行完成: %s x%d", workerID, task.GiftName, task.Count)
		}
	}
}
