package tdx

import (
	"sort"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/robfig/cron/v3"
)

type Updater interface {
	Update() error
}

func NewTimer(spec string, retry int, up Updater) error {
	//立即更新
	err := up.Update()
	if err != nil {
		return err
	}
	cr := cron.New(cron.WithSeconds())
	// 通过 spec 控制具体更新时间, 部分数据太早拉取会拿不到当天结果。
	_, err = cr.AddFunc(spec, func() {
		for i := 0; i == 0 || i < retry; i++ {
			if err := up.Update(); err != nil {
				logs.Err(err)
				<-time.After(time.Minute * 5)
			} else {
				break
			}
		}
	})
	if err != nil {
		return err
	}
	cr.Start()
	return nil
}

// NewUpdated 更新 hour=[9|15] minute=0.
// 额外的 hour/minute 成对传入, 用于一天内多个新鲜度检查节点。
func NewUpdated(db *xorms.Engine, hour, minute int, extraHourMinutes ...int) (*Updated, error) {
	err := db.Sync2(new(UpdateModel))
	u := &Updated{db: db}
	u.nodes = append(u.nodes, updateNode{hour: hour, minute: minute})
	for i := 0; i+1 < len(extraHourMinutes); i += 2 {
		u.nodes = append(u.nodes, updateNode{hour: extraHourMinutes[i], minute: extraHourMinutes[i+1]})
	}
	sort.Slice(u.nodes, func(i, j int) bool {
		if u.nodes[i].hour == u.nodes[j].hour {
			return u.nodes[i].minute < u.nodes[j].minute
		}
		return u.nodes[i].hour < u.nodes[j].hour
	})
	return u, err
}

type Updated struct {
	db    *xorms.Engine
	nodes []updateNode
}

type updateNode struct {
	hour   int
	minute int
}

func (this *Updated) Update(key string) error {
	_, err := this.db.Where("`Key`=?", key).Update(&UpdateModel{Time: time.Now().Unix()})
	return err
}

func (this *Updated) Updated(key string) (bool, error) {
	update := new(UpdateModel)
	{ //查询或者插入一条数据
		has, err := this.db.Where("`Key`=?", key).Get(update)
		if err != nil {
			return true, err
		} else if !has {
			update.Key = key
			if _, err = this.db.Insert(update); err != nil {
				return true, err
			}
			return false, nil
		}
	}
	{ //判断是否更新过,更新过则不更新
		now := time.Now()
		node := this.latestNode(now)
		updateTime := time.Unix(update.Time, 0)
		if updateTime.Before(node) {
			return false, nil
		}
	}
	return true, nil
}

func (this *Updated) latestNode(now time.Time) time.Time {
	if len(this.nodes) == 0 {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	}
	for i := len(this.nodes) - 1; i >= 0; i-- {
		node := time.Date(now.Year(), now.Month(), now.Day(), this.nodes[i].hour, this.nodes[i].minute, 0, 0, time.Local)
		if !now.Before(node) {
			return node
		}
	}
	last := this.nodes[len(this.nodes)-1]
	return time.Date(now.Year(), now.Month(), now.Day()-1, last.hour, last.minute, 0, 0, time.Local)
}

/*



 */

type UpdateModel struct {
	Key  string
	Time int64 //更新时间
}

func (*UpdateModel) TableName() string {
	return "update"
}
