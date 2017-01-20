package worker

import (
        //"os"
        "sync"
        "errors"
)

// Result represents a delivery of a job done.
type Result interface {}

// Job represents a piece of job to be done.
type Job interface {
        Action() Result
}

// Continual job
type Continue Job

// Sentry is used to ensure a sequence of jobs are done at a point.
type Sentry struct {
        worker *Worker
        mutex *sync.Mutex
        results []Result
        waiter *sync.WaitGroup
}

type guard struct {
        sentry *Sentry
        job Job
}
func (m *guard) Action() Result {
        return &unguard{ m.sentry, m.job.Action() }
}

type unguard struct {
        sentry *Sentry
        result Result
}
func (m *unguard) Action() Result {
        result := chainJob(m.result)
        m.sentry.mutex.Lock() 
        m.sentry.results = append(m.sentry.results, result)
        m.sentry.mutex.Unlock()
        m.sentry.waiter.Done()
        return result
}

type stop struct {
        waiter *sync.WaitGroup
}
func (m *stop) Action() Result { return nil }

// Worker represents a worker to dispatch jobs being done.
type Worker struct {
        routines int
        i chan Job
        o chan Result
}

// New creates a new worker valid to do user-defined jobs.
func New() *Worker {
        return &Worker{}
}

// New creates a new worker and spawn N threads to do user-defined jobs.
func SpawnN(num int) *Worker {
        w := New()
        w.SpawnN(num)
        return w
}

func chainJob(result Result) Result {
        for ; result != nil; {
                if j, ok := result.(Continue); ok && j != nil {
                        result = j.Action()
                } else {
                        break
                }
        }
        return result
}

func (w *Worker) routine(num int) {
        for msg := range w.i {
                if msg != nil {
                        var sw *sync.WaitGroup
                        if stop, ok := msg.(*stop); ok && stop != nil {
                                sw = stop.waiter
                        }
                        w.o <- msg.Action(); go w.advance(sw)
                        if sw != nil { return }
                }
        }
}

func (w *Worker) advance(wg *sync.WaitGroup) {
        if chainJob(<-w.o); wg != nil {
                wg.Done()
        }
}

// Worker.SpawnN starts a number of `num` threads for jobs.
func (w *Worker) SpawnN(num int) error {
        if w.i != nil {
                return errors.New("worker is busy on jobs")
        }
        if w.o != nil {
                return errors.New("worker is busy on job results")
        }
        w.i, w.o, w.routines = make(chan Job, 1), make(chan Result, 1), num
        for i := 0; i < num; i++ {
                go w.routine(i)
        }
        return nil
}

// Worker.Kill stops all threads.
func (w *Worker) Kill() error {
        // TODO: stops all threads immediately
        return w.Wait()
}

func (w *Worker) Wait() error {
        if w.i == nil {
                return errors.New("worker free")
        }
        if w.o == nil {
                return errors.New("worker don't have job results")
        }

        c := &stop{ new(sync.WaitGroup) }
        c.waiter.Add(w.routines)
        for i := 0; i < w.routines; i++ {
                w.i <- c
        }
        c.waiter.Wait()
        close(w.i)
        close(w.o)
        w.i, w.o = nil, nil
        return nil
}

// Worker.Do perform a job.
func (w *Worker) Do(m Job) {
        if w.i != nil {
                w.i <- m
        }
}

// Set a new sentry for the worker.
func (w *Worker) Sentry() *Sentry {
        if w.i == nil || w.o == nil {
                return nil
        }
        sentry := &Sentry{ 
                worker: w, 
                mutex: new(sync.Mutex),
                waiter: new(sync.WaitGroup),
        }
        return sentry
}

// Perform a guarded job by the sentry.
func (s *Sentry) Guard(m Job) {
        s.waiter.Add(1)
        s.worker.Do(&guard{ s, m })
}

// Wait for finish of all guarded jobs. 
func (s *Sentry) Wait() (results []Result) {
        s.waiter.Wait()
        return s.results
}
