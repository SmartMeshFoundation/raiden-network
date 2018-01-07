package blockchain

import (
	"time"

	"sync"

	"fmt"

	"context"

	"errors"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
)

//stop this call back when return non nil error
type AlarmCallback func(blockNumber int64) error

//Task to notify when a block is mined.
type AlarmTask struct {
	client          *ethclient.Client //todo race condition and reconnect, wrapper?
	lastBlockNumber int64
	shouldStop      chan struct{}
	waitTime        time.Duration
	callback        []AlarmCallback
	lock            sync.Mutex
}

func NewAlarmTask(client *ethclient.Client) *AlarmTask {
	t := &AlarmTask{
		client:          client,
		waitTime:        time.Second,
		lastBlockNumber: -1,
		shouldStop:      make(chan struct{}), //sync channel
	}
	return t
}

/*
Register a new callback.

        Note:
            The callback will be executed in the AlarmTask context and for
            this reason it should not block, otherwise we can miss block
            changes.
*/
func (this *AlarmTask) RegisterCallback(callback AlarmCallback) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.callback = append(this.callback, callback)
}

//Remove callback from the list of callbacks if it exists
func (this *AlarmTask) RemoveCallback(cb AlarmCallback) {
	this.lock.Lock()
	defer this.lock.Unlock()
	for k, c := range this.callback {
		addr1 := &c
		addr2 := &cb
		if addr1 == addr2 {
			this.callback = append(this.callback[:k], this.callback[k+1:]...)
		}
	}

}

func (this *AlarmTask) run() {
	log.Debug(fmt.Sprintf("starting block number blocknubmer=%d", this.lastBlockNumber))
	for {
		err := this.waitNewBlock()
		if err != nil {
			time.Sleep(this.waitTime)
		}
	}
}

func (this *AlarmTask) waitNewBlock() error {
	currentBlock := this.lastBlockNumber
	headerCh := make(chan *types.Header, 1)
	//get the lastest number imediatelly
	h, err := this.client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return err
	}
	headerCh <- h
	sub, err := this.client.SubscribeNewHead(context.Background(), headerCh)
	if err != nil {
		//reconnect?
		log.Warn("SubscribeNewHead block number err:", err)
		return err
	}
	for {
		select {
		case h, ok := <-headerCh:
			if !ok {
				//client broke?
				return errors.New("SubscribeNewHead channel closed unexpected")
			} else {
				if currentBlock != -1 && h.Number.Int64() != currentBlock+1 {
					log.Warn(fmt.Sprintf("alarm missed %d blocks", h.Number.Int64()-currentBlock))
				}
				currentBlock = h.Number.Int64()
				log.Trace(fmt.Sprintf("new block :%d", currentBlock))
				var removes []AlarmCallback
				for _, cb := range this.callback {
					err := cb(currentBlock)
					if err != nil {
						removes = append(removes, cb)
					}
				}
				for _, cb := range removes {
					this.RemoveCallback(cb)
				}
			}
		case <-this.shouldStop:
			sub.Unsubscribe()
			close(headerCh)
			return nil
		}

	}
	return nil
}

func (this *AlarmTask) Start() {
	go this.run()
}
func (this *AlarmTask) Stop() {
	this.shouldStop <- struct{}{}
	close(this.shouldStop)
}
