package coolq

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"github.com/sam01101/MiraiGo-qdrive/binary"
	"github.com/sam01101/MiraiGo-qdrive/message"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sam01101/gocq-qqdrive/global"
	log "github.com/sirupsen/logrus"
)

// Version go-cqhttp的版本信息，在编译时使用ldflags进行覆盖
var Version = "custom"

// CQGetLoginInfo 获取登录号信息
//
// https://git.io/Jtz1I
func (bot *CQBot) CQGetLoginInfo() MSG {
	return OK(MSG{"user_id": bot.Client.Uin, "nickname": bot.Client.Nickname})
}

func (bot *CQBot) CQUploadShortVideo(filePath string) MSG {
	_ = global.ExtractCover(filePath, filePath+".jpg")
	data, _ := ioutil.ReadFile(filePath + ".jpg")
	shortVideoElem := LocalVideoElement{
		File:  filePath,
		thumb: bytes.NewReader(data),
	}
	gv, err := bot.UploadLocalVideo(&shortVideoElem)
	if err != nil {
		log.Warnf("警告: 短视频上传失败: %v", err)
		return Failed(100, "SHORT_VIDEO_UPLOAD_FAILED", err.Error())
	}
	filename := hex.EncodeToString(gv.Md5) + ".video"
	if !global.PathExists(path.Join(global.VideoPath, filename)) {
		_ = ioutil.WriteFile(path.Join(global.VideoPath, filename), binary.NewWriterF(func(w *binary.Writer) {
			w.Write(gv.Md5)
			w.Write(gv.ThumbMd5)
			w.WriteUInt32(uint32(gv.Size))
			w.WriteUInt32(uint32(gv.ThumbSize))
			w.WriteString(gv.Name)
			w.Write(gv.Uuid)
		}), 0644)
	}
	return OK(MSG{"size": gv.Size, "file_md5": gv.Md5, "file_name": filename})
}

// CQDownloadFile 扩展API-下载文件到缓存目录
//
// https://docs.go-cqhttp.org/api/#%E4%B8%8B%E8%BD%BD%E6%96%87%E4%BB%B6%E5%88%B0%E7%BC%93%E5%AD%98%E7%9B%AE%E5%BD%95
func (bot *CQBot) CQDownloadFile(url string, headers map[string]string, threadCount int) MSG {
	hash := md5.Sum([]byte(url))
	file := path.Join(global.CachePath, hex.EncodeToString(hash[:])+".cache")
	if global.PathExists(file) {
		if err := os.Remove(file); err != nil {
			log.Warnf("删除缓存文件 %v 时出现错误: %v", file, err)
			return Failed(100, "DELETE_FILE_ERROR", err.Error())
		}
	}
	if err := global.DownloadFileMultiThreading(url, file, 0, threadCount, headers); err != nil {
		log.Warnf("下载链接 %v 时出现错误: %v", url, err)
		return Failed(100, "DOWNLOAD_FILE_ERROR", err.Error())
	}
	abs, _ := filepath.Abs(file)
	return OK(MSG{
		"file": abs,
	})
}

// CQSendGroupForwardMessage 扩展API-发送合并转发(群)
//
// https://docs.go-cqhttp.org/api/#%E5%8F%91%E9%80%81%E5%90%88%E5%B9%B6%E8%BD%AC%E5%8F%91-%E7%BE%A4
func (bot *CQBot) CQSendGroupForwardMessage(m gjson.Result) MSG {
	if m.Type != gjson.JSON {
		return Failed(100)
	}
	var sendNodes []*message.ForwardNode
	ts := time.Now().Add(-time.Minute * 5)
	var convert func(e gjson.Result) []*message.ForwardNode
	convert = func(e gjson.Result) (nodes []*message.ForwardNode) {
		if e.Get("type").Str != "node" {
			return nil
		}
		ts.Add(time.Second)
		uin, _ := strconv.ParseInt(e.Get("data.uin").Str, 10, 64)
		msgTime, err := strconv.ParseInt(e.Get("data.time").Str, 10, 64)
		if err != nil {
			msgTime = ts.Unix()
		}
		name := e.Get("data.name").Str
		c := e.Get("data.content")
		if c.IsArray() {
			flag := false
			c.ForEach(func(_, value gjson.Result) bool {
				if value.Get("type").String() == "node" {
					flag = true
					return false
				}
				return true
			})
			if flag {
				var taowa []*message.ForwardNode
				for _, item := range c.Array() {
					taowa = append(taowa, convert(item)...)
				}
				nodes = append(nodes, &message.ForwardNode{
					SenderId:   uin,
					SenderName: name,
					Time:       int32(msgTime),
					Message:    []message.IMessageElement{bot.Client.UploadForwardMessage(&message.ForwardMessage{Nodes: sendNodes})},
				})
				return
			}
		}
		content := bot.ConvertObjectMessage(e.Get("data.content"), true)
		if uin != 0 && name != "" && len(content) > 0 {
			var newElem []message.IMessageElement
			for _, elem := range content {
				if video, ok := elem.(*LocalVideoElement); ok {
					gm, err := bot.UploadLocalVideo(video)
					if err != nil {
						log.Warnf("警告：视频上传失败: %v", err)
						continue
					}
					newElem = append(newElem, gm)
					continue
				}
				newElem = append(newElem, elem)
			}
			nodes = append(nodes, &message.ForwardNode{
				SenderId:   uin,
				SenderName: name,
				Time:       int32(msgTime),
				Message:    newElem,
			})
			return
		}
		log.Warnf("警告: 非法 Forward node 将跳过")
		return
	}
	if m.IsArray() {
		for _, item := range m.Array() {
			sendNodes = append(sendNodes, convert(item)...)
		}
	} else {
		sendNodes = convert(m)
	}
	if len(sendNodes) > 0 {
		ret := bot.Client.UploadForwardMessage(&message.ForwardMessage{Nodes: sendNodes})
		if ret == nil {
			log.Warnf("合并转发(群)消息发送失败: 账号可能被风控.")
			return Failed(100, "SEND_MSG_API_ERROR", "请参考输出")
		}
		return OK(MSG{
			"message_id": ret.ResId,
		})
	}
	return Failed(100)
}

// CQGetForwardMessage 获取合并转发消息
//
// https://git.io/Jtz1F
func (bot *CQBot) CQGetForwardMessage(resID string) MSG {
	m := bot.Client.GetForwardMessage(resID)
	if m == nil {
		return Failed(100, "MSG_NOT_FOUND", "消息不存在")
	}
	r := make([]MSG, 0)
	// Send all request first, then get from store after
	for _, n := range m.Nodes {
		bot.checkMedia(n.Message, false)
		time.Sleep(time.Millisecond)
	}
	time.Sleep(time.Second * 2)
	for _, n := range m.Nodes {
		bot.checkMedia(n.Message, true)
	}
	for _, n := range m.Nodes {
		r = append(r, MSG{
			"sender": MSG{
				"user_id": n.SenderId,
			},
			"content": ToFormattedMessage(n.Message, false),
		})
	}
	return OK(MSG{
		"messages": r,
	})
}

// OK 生成成功返回值
func OK(data interface{}) MSG {
	return MSG{"data": data, "retcode": 0, "status": "ok"}
}

// Failed 生成失败返回值
func Failed(code int, msg ...string) MSG {
	m := ""
	w := ""
	if len(msg) > 0 {
		m = msg[0]
	}
	if len(msg) > 1 {
		w = msg[1]
	}
	return MSG{"data": nil, "retcode": code, "msg": m, "wording": w, "status": "failed"}
}
