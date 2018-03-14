package httpclientpool

import (
	"errors"
	"net/http"
	"runtime"
	"sync"
	"time"
)

// ErrWorkPoolStarted : call start function of workerpool twice or more
var ErrWorkPoolStarted = errors.New("workerpool already started")

// ErrWorkPoolNotStart : used in workerpool.stop function
var ErrWorkPoolNotStart = errors.New("workerpool has not been started")

// ErrWorkPoolGetCh : the maxworkercount is too small maybe
var ErrWorkPoolGetCh = errors.New("workerpool getch error")

var workerChanCap = func() int {
	if runtime.GOMAXPROCS(0) == 1 {
		return 0
	}
	return 1
}

// Job : work will do the job
type Job interface {
	Do(c *http.Client) interface{}
}

// HTTPClient ： workerpool will call it if suitable
type HTTPClient interface {
	Create() (*http.Client, error)
}

// workChan : for producer-consumer
type workChan struct {
	lastUseTime time.Time
	ch          chan *http.Client
}

// WorkerPool : http client pool
type WorkerPool struct {
	sync.Mutex
	MaxWorkerCount        int // the max worker supplied
	MaxIdleWorkerDuration time.Duration
	workerCount           int
	mustStop              bool
	stopCh                chan struct{}
	ready                 []*workChan
	workerChanPool        sync.Pool
}

// Start ： start workerpool
func (wp *WorkerPool) Start() error {
	if wp.stopCh != nil {
		return ErrWorkPoolStarted
	}
	wp.stopCh = make(chan struct{})
	stopCh := wp.stopCh
	wp.mustStop = false

	// scale the ready array
	go func() {
		for {
			wp.clean()
			select {
			case <-stopCh:
				return
			default:
				time.Sleep(wp.getMaxIdleWorkerDuration())
			}
		}
	}()
	return nil
}

// Stop : call it when want to stop pool
func (wp *WorkerPool) Stop() error {
	if wp.stopCh == nil {
		return ErrWorkPoolNotStart
	}
	close(wp.stopCh)
	wp.stopCh = nil

	wp.Lock()
	defer wp.Unlock()

	for i, ch := range wp.ready {
		ch.ch <- nil
		//close(ch.ch)
		wp.ready[i] = nil
	}
	wp.ready = wp.ready[:0]
	wp.mustStop = true
	return nil
}

func (wp *WorkerPool) clean() {
	maxIdleWorkerDuration := wp.getMaxIdleWorkerDuration()
	currentTime := time.Now()
	n := len(wp.ready)
	i := 0
	for i < n && currentTime.Sub(wp.ready[i].lastUseTime) > maxIdleWorkerDuration {
		i++
	}

	discard := make([]*workChan, i)
	discard = append(discard[:0], wp.ready[:i]...)

	for tmp := 0; tmp < i; tmp++ {
		discard[tmp].ch <- nil
		discard[tmp] = nil
	}

	wp.Lock()
	wp.ready = wp.ready[:i]
	wp.Unlock()
}

func (wp *WorkerPool) getCh() (*workerChan, error) {
	var ch *workerChan
	createWorker := false

	wp.Lock()
	n := len(wp.ready) - 1
	if n < 0 {
		if wp.workerCount < wp.MaxWorkerCount {
			createWorker = true
			wp.workerCount++
		}
	} else {
		ch = wp.ready[n]
		ready[n] = nil
		wp.ready = wp.ready[:n]
	}
	wp.Unlock

	if ch == nil {
		if !createWorker {
			return nil, ErrWorkPoolGetCh
		}
		vch := wp.workerChanPool.Get()
		if vch == nil {
			vch = &workerChan{
				ch: make(chan *http.Client, workerChanCap),
			}
		}
		ch = vch.(*workerChan)
		go func() {
			wp.workerFunc(ch)
			wp.workerChanPool.Put(vch)
		}()
	}
	return ch
}

func (wp *WorkerPool) workerFunc(ch *workerChan) {

}

func (wp *WorkerPool) getMaxIdleWorkerDuration() time.Duration {
	if wp.MaxIdleWorkerDuration <= 0 {
		return 10 * time.Second
	}
	return wp.MaxIdleWorkerDuration
}
