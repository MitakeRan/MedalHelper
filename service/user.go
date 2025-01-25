package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ThreeCatsLoveFish/medalhelper/dto"
	"github.com/ThreeCatsLoveFish/medalhelper/manager"
	"github.com/ThreeCatsLoveFish/medalhelper/service/push"
	"github.com/ThreeCatsLoveFish/medalhelper/util"

	"github.com/TwiN/go-color"
	"github.com/google/uuid"
	"github.com/sethvargo/go-retry"
)

type User struct {
	// 用户ID
	Uid int
	// 用户名称
	Name string
	// 是否登录
	isLogin bool
	// UUID
	uuid []string

	// 登录凭证
	accessKey string
	// 白名单的房间ID
	allowedUIDs []int
	// 被禁止的房间ID
	bannedUIDs []int
	// 推送服务
	pushName string

	// 用户佩戴的勋章
	wearMedal dto.MedalInfo
	// 用户等级小于20的勋章
	medalsLow []dto.MedalInfo
	// 今日亲密度没满的勋章
	remainMedals []dto.MedalInfo

	// 日志信息
	message string
}

func NewUser(accessKey, pushName string, allowUIDs, banUIDs []int) User {
	return User{
		accessKey:   accessKey,
		allowedUIDs: allowUIDs,
		bannedUIDs:  banUIDs,
		pushName:    pushName,
		wearMedal:   dto.DefaultMedal,
		uuid:        []string{uuid.NewString(), uuid.NewString()},
		message:     "",
	}
}

func (user User) info(format string, v ...interface{}) {
	format = color.Green + "[INFO] " + color.Reset + format
	format = color.Reset + color.Blue + user.Name + color.Reset + " " + format
	util.PrintColor(format, v...)
}

func (user *User) loginVerify() bool {
	resp, err := manager.LoginVerify(user.accessKey)
	if err != nil || resp.Data.Mid == 0 {
		user.isLogin = false
		return false
	}
	user.Uid = resp.Data.Mid
	user.Name = resp.Data.Name
	user.isLogin = true
	user.info("登录成功")
	return true
}

func (user *User) setMedals() {
	// Clean medals storage
	user.medalsLow = make([]dto.MedalInfo, 0, 10)
	user.remainMedals = make([]dto.MedalInfo, 0, 10)
	// Fetch and update medals
	medals, wearMedal := manager.GetMedal(user.accessKey)
	if wearMedal {
		user.wearMedal = medals[0]
	}
	// Whitelist
	if len(user.allowedUIDs) > 0 {
		for _, medal := range medals {
			if util.IntContain(user.allowedUIDs, medal.Medal.TargetID) != -1 {
				user.medalsLow = append(user.medalsLow, medal)
				if medal.Medal.Level < 20 && medal.Medal.TodayFeed < 1500 || medal.Medal.TodayFeed < 300 {
					user.remainMedals = append(user.remainMedals, medal)
					user.info(fmt.Sprintf("%s 在白名单中，加入任务", medal.AnchorInfo.NickName))
				}
			}
		}
		return
	}
	// Default blacklist
	for _, medal := range medals {
		if util.IntContain(user.bannedUIDs, medal.Medal.TargetID) != -1 {
			continue
		}
		if medal.RoomInfo.RoomID == 0 {
			continue
		}
		if medal.Medal.Level <= 20 {
			user.medalsLow = append(user.medalsLow, medal)
			if medal.Medal.TodayFeed < 1500 {
				user.remainMedals = append(user.remainMedals, medal)
			}
		}
	}
}

func (user *User) checkMedals() bool {
	user.setMedals()
	fullMedalList := make([]string, 0, len(user.medalsLow))
	failMedalList := make([]string, 0)
	for _, medal := range user.medalsLow {
		if medal.Medal.TodayFeed == 1500 {
			fullMedalList = append(fullMedalList, medal.AnchorInfo.NickName)
		} else {
			failMedalList = append(failMedalList, medal.AnchorInfo.NickName)
		}
	}
	user.message = fmt.Sprintf(
		"20级以下牌子共 %d 个\n【1500】 %v等 %d个\n【1500以下】 %v等 %d个\n",
		len(user.medalsLow), fullMedalList, len(fullMedalList),
		failMedalList, len(failMedalList),
	)
	if user.wearMedal != (dto.MedalInfo{}) {
		user.message += fmt.Sprintf(
			"【当前佩戴】「%s」(%s) %d 级 \n",
			user.wearMedal.Medal.MedalName,
			user.wearMedal.AnchorInfo.NickName, user.wearMedal.Medal.Level,
		)
		if user.wearMedal.Medal.Level < 20 && user.wearMedal.Medal.TodayFeed != 0 {
			need := user.wearMedal.Medal.NextIntimacy - user.wearMedal.Medal.Intimacy
			needDays := need/1500 + 1
			endDate := time.Now().AddDate(0, 0, needDays)
			user.message += fmt.Sprintf(
				"今日已获取亲密度 %d (B站结算有延迟，请耐心等待)\n距离下一级还需 %d 亲密度 预计需要 %d 天 (%s,以每日 1500 亲密度计算)\n",
				user.wearMedal.Medal.TodayFeed, need,
				needDays, endDate.Format("2006-01-02"),
			)
		}
	}
	user.info(user.message)
	return len(fullMedalList) == len(user.medalsLow)
}

// Send daily report notification
func (user *User) report() {
	if len(user.pushName) != 0 {
		pushEnd := push.NewPush(user.pushName)
		_ = pushEnd.Submit(push.Data{
			Title:   "# 今日亲密度获取情况如下",
			Content: fmt.Sprintf("用户%s，%s", user.Name, user.message),
		})
	}
}

// Send expire notification
func (user *User) expire() {
	if len(user.pushName) != 0 {
		pushEnd := push.NewPush(user.pushName)
		_ = pushEnd.Submit(push.Data{
			Title:   "# AccessKey 过期",
			Content: fmt.Sprintf("用户未登录, accessKey: %s", user.accessKey),
		})
	}
}

func (user *User) Init() bool {
	if user.loginVerify() {
		user.setMedals()
		return true
	} else {
		util.Error("用户登录失败, accessKey: %s", user.accessKey)
		user.expire()
		return false
	}
}

func (user *User) RunOnce() bool {
	switch util.GlobalConfig.CD.Async {
	case 0: // Sync
		task := NewTask(*user, []IAction{
			&SyncWatchLive{},
		})
		task.Start()
	case 1: // Async
		task := NewTask(*user, []IAction{
			&AsyncWatchLive{},
		})
		task.Start()
	}
	return user.checkMedals()
}

func (user *User) Start(wg *sync.WaitGroup) {
	if user.isLogin {
		backOff := retry.NewConstant(5 * time.Second)
		backOff = retry.WithMaxRetries(3, backOff)
		_ = retry.Do(context.Background(), backOff, func(ctx context.Context) error {
			if ok := user.RunOnce(); !ok {
				return retry.RetryableError(errors.New("task not complete"))
			}
			return nil
		})
		user.report()
	} else {
		util.Error("用户未登录, accessKey: %s", user.accessKey)
	}
	wg.Done()
}
