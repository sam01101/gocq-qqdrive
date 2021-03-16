package coolq

import (
	"github.com/sam01101/MiraiGo-qdrive/message"
	log "github.com/sirupsen/logrus"
)

var format = "string"

// SetMessageFormat 设置消息上报格式，默认为string
func SetMessageFormat(f string) {
	format = f
}

func (bot *CQBot) checkMedia(e []message.IMessageElement, getFromSeq bool) {
	for _, elem := range e {
		switch i := elem.(type) {
		case *message.ShortVideoElement:
			if getFromSeq {
				url, err := bot.Client.GetSeq(i.Seq)
				if err != nil {
					log.Errorf("Error while getting short video url: %v", err)
					i.Url = ""
				} else {
					i.Url = url.(string)
				}
			} else {
				i.Seq = bot.Client.GetShortVideoUrlSeq(i.Uuid, i.Md5)
			}
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
