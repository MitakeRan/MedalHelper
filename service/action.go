package service

import (
	"time"

	"github.com/ThreeCatsLoveFish/medalhelper/dto"
	"github.com/ThreeCatsLoveFish/medalhelper/manager"
)

type SyncWatchLive struct {
	WatchLive
	SyncAction
}

type AsyncWatchLive struct {
	WatchLive
	AsyncAction
}

// WatchLive implement IExec, default sync, include sending heartbeat

type WatchLive struct{}

func (WatchLive) Do(user User, medal dto.MedalInfo, n int) bool {
	var times int
	if medal.Medal.Level < 20 {
		times = 25
	} else {
		times = 5
	}
	for i := 1; i <= times; i++ {
		if ok := manager.Heartbeat(
			user.accessKey,
			user.uuid,
			medal.RoomInfo.RoomID,
			medal.Medal.TargetID,
		); !ok {
			return false
		}
		if i%5 == 0 {
			user.info("%s 第%d次心跳包已发送(%d/%d)", medal.AnchorInfo.NickName, i, n, len(user.remainMedals))
		}
		time.Sleep(1 * time.Minute)
	}
	return true
}

func (WatchLive) Finish(user User, medal []dto.MedalInfo) {
	if len(medal) == 0 {
		user.info("每日25分钟完成")
	} else {
		user.info("每日25分钟未完成,剩余(%d/%d)", len(medal), len(user.medalsLow))
	}
}
