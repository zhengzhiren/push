package comet

type Worker struct {
	id         int
	ctrl       chan bool
	jobChannel chan Job
}

type Job interface {
	Do() bool
}

func NewWorker(id int) *Worker {
	return &Worker{
		id:         id,
		ctrl:       make(chan bool),
		jobChannel: make(chan Job, 1000),
	}
}

func (this *Worker) Run() {
	go func() {
		//log.Infof("send routine %d: start", this.id)
		for {
			select {
			case job := <-this.jobChannel:
				job.Do()
				//time.Sleep(10 * time.Millisecond)
			case <-this.ctrl:
				//log.Infof("job routine %d: stop", this.id)
				return
			}
		}
	}()
}

func (this *Worker) Stop() {
	close(this.ctrl)
}
