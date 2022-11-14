package queuemanager

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path"
	"sync"
)

type queueManager struct {
	mtx      sync.RWMutex
	Options  Options
	Workers  map[string]func(ctx context.Context, id string, data io.Reader) error
	queue    []task
	position int
	cond     sync.Cond
	decoder  Decoder
}

type task struct {
	file string
	id   string
	do   func(ctx context.Context, id string, data io.Reader) error
}

type Options struct {
	Prefix string
}

type Decoder interface {
	DecodeCSV(ctx context.Context, id string, data io.Reader) error
	DecodeJSONBatch(ctx context.Context, id string, data io.Reader) error
}

func New(decoder Decoder) *queueManager {
	qm := &queueManager{
		decoder: decoder,
		Workers: make(map[string]func(ctx context.Context, id string, data io.Reader) error),
	}
	qm.cond = *sync.NewCond(&qm.mtx)
	return qm
}

// Start: starts up quemanager
func (qm *queueManager) Start(ctx context.Context) {
	go func() {
		for {
			task := qm.getTask()
			qm.position++
			go func() {
				log.Println("queue manager got new task")
				file, err := os.Open(path.Join(qm.Options.Prefix, task.file))
				if err != nil {
					log.Println(err)
				}
				defer file.Close()
				err = task.do(ctx, task.id, file)
				if err != nil {
					log.Println(err)
				}
			}()
		}
	}()
}

// getTask: returns task first in queue if there isnt any it awaits a signal
func (qm *queueManager) getTask() task {
	qm.mtx.Lock()
	defer qm.mtx.Unlock()

	if len(qm.queue)-qm.position < 1 {
		qm.cond.Wait()
	}

	return qm.queue[qm.position]
}

// PushTask: pushes new task with ID, type, name and byte body then wakes up wait routine
func (qm *queueManager) PushTask(id string, taskType string, body []byte, name string) error {
	go func() {
		qm.mtx.Lock()
		defer qm.mtx.Unlock()

		worker, fd := qm.Workers[taskType]
		if !fd {
			log.Println(errors.New("data is in unknown format"))
			return
		}
		file, err := os.Create(path.Join(qm.Options.Prefix, name))
		if err != nil {
			log.Println(err.Error())
			return
		}
		_, err = file.Write(body)
		if err != nil {
			log.Println(err.Error())
			return
		}
		file.Close()

		qm.queue = append(qm.queue, task{id: id, file: name, do: worker})
		qm.cond.Signal()
	}()
	return nil
}
