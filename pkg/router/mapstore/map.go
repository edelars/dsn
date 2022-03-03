package mapstore

import (
	"context"
	"sync"

	uuid "github.com/gofrs/uuid"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/model"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/worker"
)

//Store -> ItemStore -> itemAggregatorArray

type Store struct {
	sync.Mutex
	idList map[uuid.UUID]*ItemStore
	isStop bool
}

func NewStore() *Store {
	return &Store{
		idList: make(map[uuid.UUID]*ItemStore),
		isStop: false,
	}
}

//Return true if item exist, or false if new item was created.
//Return r *ItemStore == nil if GetAllWorkerCancelArray() was called and we going to die ;(
func (s *Store) GetOrCreate(id uuid.UUID, newItemStore *ItemStore, respMessagechan *chan *model.ResponseMessage, ctx context.Context) (r *ItemStore, exist bool) {
	s.Lock()
	defer s.Unlock()

	if s.isStop {
		return nil, true
	}

	if r, exist = s.idList[id]; !exist {
		newItemStore.store = s
		newItemStore.itemsAggregatorArray = append(newItemStore.itemsAggregatorArray, *NewItemAggregatorArray(respMessagechan, ctx))
		s.idList[id] = newItemStore
		return newItemStore, exist

	} else {
		r.itemsAggregatorArray = append(r.itemsAggregatorArray, *NewItemAggregatorArray(respMessagechan, ctx))
		return r, exist
	}

}

//Make Unlock on ItemStore before delete. Needed to Lock Store before call Delete()
func (s *Store) Delete(id uuid.UUID) {
	defer s.Unlock()
	delete(s.idList, id)
}

//Calling this function is block Store on forever. Need shutdown service after this
func (s *Store) GetAllWorkerCancelArray() []context.CancelFunc {
	s.Lock()
	defer s.Unlock()
	s.isStop = true
	var r []context.CancelFunc
	for _, v := range s.idList {
		r = append(r, v.workerCancel)
	}
	return r
}

//Make Lock on Store
func (s *Store) PreFlight() {
	s.Lock()
}

//Make Unlock on Store
func (s *Store) AfterFlight() {
	s.Unlock()
}

type ItemStore struct {
	itemsAggregatorArray []itemAggregatorArray
	Worker               *worker.RequesterStatusHandler
	WorkerChan           chan *model.ResponseMessage
	workerCancel         context.CancelFunc
	store                *Store
}

func NewItemStore(worker *worker.RequesterStatusHandler, workerCancel context.CancelFunc) *ItemStore {
	return &ItemStore{
		itemsAggregatorArray: []itemAggregatorArray{},
		Worker:               worker,
		WorkerChan:           make(chan *model.ResponseMessage, 5),
		workerCancel:         workerCancel,
	}
}

//Needed to call PreFlight() before and Unlock with AfterFlight() or Delete().
//Return array of chan only with open status. Chan with "closed"/true status will be deleted
func (i *ItemStore) GetAggregatorChanArray() []*chan *model.ResponseMessage {

	var (
		aggregatorChanArray     []*chan *model.ResponseMessage
		newItemsAggregatorArray []itemAggregatorArray
	)

	for _, v := range i.itemsAggregatorArray {
		if ch, closed := v.GetAggregatorChan(); !closed {
			aggregatorChanArray = append(aggregatorChanArray, ch)
			newItemsAggregatorArray = append(newItemsAggregatorArray, v)
		}
	}

	i.itemsAggregatorArray = newItemsAggregatorArray

	return aggregatorChanArray
}

func (i *ItemStore) GetWorkerCancel() context.CancelFunc {
	return i.workerCancel
}

type itemAggregatorArray struct {
	aggregatorChan *chan *model.ResponseMessage
	ctx            context.Context
}

func NewItemAggregatorArray(aggregatorChan *chan *model.ResponseMessage, ctx context.Context) *itemAggregatorArray {
	return &itemAggregatorArray{
		aggregatorChan: aggregatorChan,
		ctx:            ctx,
	}
}

//Return true if chan is closed
func (i *itemAggregatorArray) GetAggregatorChan() (*chan *model.ResponseMessage, bool) {
	return i.aggregatorChan, i.ctx.Err() != nil
}
