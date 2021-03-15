package coolq

import (
	"encoding/hex"
	"github.com/sam01101/MiraiGo-qdrive/binary"
	"github.com/sam01101/MiraiGo-qdrive/message"
	"github.com/sam01101/gocq-qqdrive/global"
	"io/ioutil"
	"path"
)

var format = "string"

// SetMessageFormat 设置消息上报格式，默认为string
func SetMessageFormat(f string) {
	format = f
}

func (bot *CQBot) checkMedia(e []message.IMessageElement) {
	for _, elem := range e {
		switch i := elem.(type) {
		case *message.ShortVideoElement:
			filename := hex.EncodeToString(i.Md5) + ".video"
			if !global.PathExists(path.Join(global.VideoPath, filename)) {
				_ = ioutil.WriteFile(path.Join(global.VideoPath, filename), binary.NewWriterF(func(w *binary.Writer) {
					w.Write(i.Md5)
					w.Write(i.ThumbMd5)
					w.WriteUInt32(uint32(i.Size))
					w.WriteUInt32(uint32(i.ThumbSize))
					w.WriteString(i.Name)
					w.Write(i.Uuid)
				}), 0644)
			}
			i.Name = filename
			i.Url = bot.Client.GetShortVideoUrl(i.Uuid, i.Md5)
		}
	}
}

// ToFormattedMessage 将给定[]message.IMessageElement转换为通过coolq.SetMessageFormat所定义的消息上报格式
func ToFormattedMessage(e []message.IMessageElement, isRaw ...bool) (r interface{}) {
	if format == "string" {
		r = ToStringMessage(e, isRaw...)
	} else if format == "array" {
		r = ToArrayMessage(e, isRaw...)
	}
	return
}
