package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ThreeCatsLoveFish/medalhelper/dto"
	"github.com/ThreeCatsLoveFish/medalhelper/util"

	"github.com/sethvargo/go-retry"
)

// SyncAction implement IConcurrency, support synchronous actions
type SyncAction struct{}

func (a *SyncAction) Exec(user User, job *sync.WaitGroup, child IExec) []dto.MedalInfo {
	fail := make([]dto.MedalInfo, 0, len(user.medalsLow)+len(user.medals20))
	for i, medal := range user.remainMedals {
		retryTime := util.GlobalConfig.CD.Retry
		if retryTime == 0 {
			if ok := child.Do(user, medal, i+1); !ok {
				fail = append(fail, medal)
			}
		} else {
			backOff := retry.NewFibonacci(time.Duration(retryTime) * time.Second)
			backOff = retry.WithMaxRetries(uint64(util.GlobalConfig.CD.MaxTry), backOff)
			err := retry.Do(context.Background(), backOff, func(ctx context.Context) error {
				if ok := child.Do(user, medal, i+1); !ok {
					return retry.RetryableError(errors.New("action fail"))
				}
				return nil
			})
			if err != nil {
				fail = append(fail, medal)
			}
		}
	}
	for i, medal := range user.remainMedals20 {
		retryTime := util.GlobalConfig.CD.Retry
		if retryTime == 0 {
			if ok := child.Do(user, medal, len(user.remainMedals)+i+1); !ok {
				fail = append(fail, medal)
			}
		} else {
			backOff := retry.NewFibonacci(time.Duration(retryTime) * time.Second)
			backOff = retry.WithMaxRetries(uint64(util.GlobalConfig.CD.MaxTry), backOff)
			err := retry.Do(context.Background(), backOff, func(ctx context.Context) error {
				if ok := child.Do(user, medal, len(user.remainMedals)+i+1); !ok {
					return retry.RetryableError(errors.New("action fail"))
				}
				return nil
			})
			if err != nil {
				fail = append(fail, medal)
			}
		}
	}
	child.Finish(user, fail)
	job.Done()
	return fail
}

// AsyncAction implement IConcurrency, support asynchronous actions
type AsyncAction struct{}

func (a *AsyncAction) Exec(user User, job *sync.WaitGroup, child IExec) []dto.MedalInfo {
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	fail := make([]dto.MedalInfo, 0, len(user.medalsLow))
	for i, medal := range user.remainMedals {
		wg.Add(1)
		retryTime := util.GlobalConfig.CD.Retry
		if retryTime == 0 {
			go func(medal dto.MedalInfo) {
				if ok := child.Do(user, medal, i+1); !ok {
					mu.Lock()
					fail = append(fail, medal)
					mu.Unlock()
				}
				wg.Done()
			}(medal)
		} else {
			backOff := retry.NewFibonacci(time.Duration(retryTime) * time.Second)
			backOff = retry.WithMaxRetries(uint64(util.GlobalConfig.CD.MaxTry), backOff)
			go func(medal dto.MedalInfo) {
				err := retry.Do(context.Background(), backOff, func(ctx context.Context) error {
					if ok := child.Do(user, medal, i+1); !ok {
						return retry.RetryableError(errors.New("action fail"))
					}
					return nil
				})
				if err != nil {
					mu.Lock()
					fail = append(fail, medal)
					mu.Unlock()
				}
				wg.Done()
			}(medal)
		}
	}
	wg.Wait()
	child.Finish(user, fail)
	job.Done()
	return fail
}
