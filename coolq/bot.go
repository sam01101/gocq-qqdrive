package coolq

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path"
	"runtime/debug"
	"time"

	"github.com/Mrs4s/MiraiGo/binary"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/Mrs4s/MiraiGo/utils"
	"github.com/Mrs4s/go-cqhttp/global"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// CQBot CQBot结构体,存储Bot实例相关配置
type CQBot struct {
	Client *client.QQClient

	events []func(MSG)
	db     *leveldb.DB
}

// MSG 消息Map
type MSG map[string]interface{}

// ForceFragmented 是否启用强制分片
var ForceFragmented = false

// NewQQBot 初始化一个QQBot实例
func NewQQBot(cli *client.QQClient, conf *global.JSONConfig) *CQBot {
	bot := &CQBot{
		Client: cli,
	}
	go func() {
		i := conf.HeartbeatInterval
		if i < 0 {
			log.Warn("警告: 心跳功能已关闭，若非预期，请检查配置文件。")
			return
		}
		if i == 0 {
			i = 5
		}
		for {
			time.Sleep(time.Second * i)
			bot.dispatchEventMessage(MSG{
				"time":            time.Now().Unix(),
				"self_id":         bot.Client.Uin,
				"post_type":       "meta_event",
				"meta_event_type": "heartbeat",
				"interval":        1000 * i,
			})
		}
	}()
	return bot
}

// OnEventPush 注册事件上报函数
func (bot *CQBot) OnEventPush(f func(m MSG)) {
	bot.events = append(bot.events, f)
}

// UploadLocalVideo 上传本地短视频至群聊
func (bot *CQBot) UploadLocalVideo(target int64, v *LocalVideoElement) (*message.ShortVideoElement, error) {
	if v.File != "" {
		video, err := os.Open(v.File)
		if err != nil {
			return nil, err
		}
		defer video.Close()
		hash, _ := utils.ComputeMd5AndLength(io.MultiReader(video, v.thumb))
		cacheFile := path.Join(global.CachePath, hex.EncodeToString(hash[:])+".cache")
		_, _ = video.Seek(0, io.SeekStart)
		_, _ = v.thumb.Seek(0, io.SeekStart)
		return bot.Client.UploadGroupShortVideo(target, video, v.thumb, cacheFile)
	}
	return &v.ShortVideoElement, nil
}

// SendGroupMessage 发送群消息
func (bot *CQBot) SendGroupMessage(groupID int64, m *message.SendingMessage) int32 {
	var newElem []message.IMessageElement
	for _, elem := range m.Elements {
		if i, ok := elem.(*LocalVideoElement); ok {
			gv, err := bot.UploadLocalVideo(0, i)
			if err != nil {
				log.Warnf("警告: 群 %v 消息短视频上传失败: %v", groupID, err)
				continue
			}
			newElem = append(newElem, gv)
			continue
		}
		newElem = append(newElem, elem)
	}
	if len(newElem) == 0 {
		log.Warnf("群消息发送失败: 消息为空.")
		return -1
	}
	m.Elements = newElem
	//bot.checkMedia(newElem)
	ret := bot.Client.SendGroupMessage(groupID, m, ForceFragmented)
	if ret == nil || ret.Id == -1 {
		log.Warnf("群消息发送失败: 账号可能被风控.")
		return -1
	}
	return bot.InsertGroupMessage(ret)
}

// InsertGroupMessage 群聊消息入数据库
func (bot *CQBot) InsertGroupMessage(m *message.GroupMessage) int32 {
	val := MSG{
		"message-id":  m.Id,
		"internal-id": m.InternalId,
		"group":       m.GroupCode,
		"group-name":  m.GroupName,
		"sender":      m.Sender,
		"time":        m.Time,
		"message":     ToStringMessage(m.Elements, m.GroupCode, true),
	}
	id := toGlobalID(m.GroupCode, m.Id)
	if bot.db != nil {
		buf := new(bytes.Buffer)
		if err := gob.NewEncoder(buf).Encode(val); err != nil {
			log.Warnf("记录聊天数据时出现错误: %v", err)
			return -1
		}
		if err := bot.db.Put(binary.ToBytes(id), binary.GZipCompress(buf.Bytes()), nil); err != nil {
			log.Warnf("记录聊天数据时出现错误: %v", err)
			return -1
		}
	}
	return id
}

// toGlobalID 构建`code`-`msgID`的字符串并返回其CRC32 Checksum的值
func toGlobalID(code int64, msgID int32) int32 {
	return int32(crc32.ChecksumIEEE([]byte(fmt.Sprintf("%d-%d", code, msgID))))
}

// Release 释放Bot实例
func (bot *CQBot) Release() {
	if bot.db != nil {
		_ = bot.db.Close()
	}
}

func (bot *CQBot) dispatchEventMessage(m MSG) {
	if global.EventFilter != nil && !global.EventFilter.Eval(global.MSG(m)) {
		log.Debug("Event filtered!")
		return
	}
	for _, f := range bot.events {
		go func(fn func(MSG)) {
			defer func() {
				if pan := recover(); pan != nil {
					log.Warnf("处理事件 %v 时出现错误: %v \n%s", m, pan, debug.Stack())
				}
			}()
			start := time.Now()
			fn(m)
			end := time.Now()
			if end.Sub(start) > time.Second*5 {
				log.Debugf("警告: 事件处理耗时超过 5 秒 (%v), 请检查应用是否有堵塞.", end.Sub(start))
			}
		}(f)
	}
}

// ToJSON 生成JSON字符串
func (m MSG) ToJSON() string {
	b, _ := json.Marshal(m)
	return string(b)
}
