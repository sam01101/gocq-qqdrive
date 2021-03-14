package coolq

import (
	"bytes"
	"crypto/md5"
	goBinary "encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/Mrs4s/MiraiGo/binary"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/Mrs4s/MiraiGo/utils"
	"github.com/Mrs4s/go-cqhttp/global"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

/*
var matchReg = regexp.MustCompile(`\[CQ:\w+?.*?]`)
var typeReg = regexp.MustCompile(`\[CQ:(\w+)`)
var paramReg = regexp.MustCompile(`,([\w\-.]+?)=([^,\]]+)`)
*/

// IgnoreInvalidCQCode 是否忽略无效CQ码
var IgnoreInvalidCQCode = true

// SplitURL 是否分割URL
var SplitURL = false

// magicCQ 代表 uint32([]byte("[CQ:"))
var magicCQ = uint32(0)

func init() {
	const sizeInt = int(unsafe.Sizeof(0))
	x := 0x1234
	p := unsafe.Pointer(&x)
	p2 := (*[sizeInt]byte)(p)
	if p2[0] == 0 {
		magicCQ = goBinary.BigEndian.Uint32([]byte("[CQ:"))
	} else {
		magicCQ = goBinary.LittleEndian.Uint32([]byte("[CQ:"))
	}
}

// add 指针运算
func add(ptr unsafe.Pointer, offset uintptr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(ptr) + offset)
}

const maxVideoSize = 1024 * 1024 * 100 // 100MB

// LocalVideoElement 本地视频
type LocalVideoElement struct {
	message.ShortVideoElement
	File  string
	thumb io.ReadSeeker
}

// ToArrayMessage 将消息元素数组转为MSG数组以用于消息上报
func ToArrayMessage(e []message.IMessageElement, id int64, isRaw ...bool) (r []MSG) {
	r = []MSG{}
	ur := false
	if len(isRaw) != 0 {
		ur = isRaw[0]
	}
	m := &message.SendingMessage{Elements: e}
	reply := m.FirstOrNil(func(e message.IMessageElement) bool {
		_, ok := e.(*message.ReplyElement)
		return ok
	})
	if reply != nil {
		r = append(r, MSG{
			"type": "reply",
			"data": map[string]string{"id": fmt.Sprint(toGlobalID(id, reply.(*message.ReplyElement).ReplySeq))},
		})
	}
	for _, elem := range e {
		var m MSG
		switch o := elem.(type) {
		case *message.TextElement:
			m = MSG{
				"type": "text",
				"data": map[string]string{"text": o.Content},
			}
		case *message.LightAppElement:
			// m = MSG{
			// 	"type": "text",
			// 	"data": map[string]string{"text": o.Content},
			// }
			m = MSG{
				"type": "json",
				"data": map[string]string{"data": o.Content},
			}
		case *message.AtElement:
			if o.Target == 0 {
				m = MSG{
					"type": "at",
					"data": map[string]string{"qq": "all"},
				}
			} else {
				m = MSG{
					"type": "at",
					"data": map[string]string{"qq": fmt.Sprint(o.Target)},
				}
			}
		case *message.RedBagElement:
			m = MSG{
				"type": "redbag",
				"data": map[string]string{"title": o.Title},
			}
		case *message.ForwardElement:
			m = MSG{
				"type": "forward",
				"data": map[string]string{"id": o.ResId},
			}
		case *message.FaceElement:
			m = MSG{
				"type": "face",
				"data": map[string]string{"id": fmt.Sprint(o.Index)},
			}
		case *message.VoiceElement:
			if ur {
				m = MSG{
					"type": "record",
					"data": map[string]string{"file": o.Name},
				}
			} else {
				m = MSG{
					"type": "record",
					"data": map[string]string{"file": o.Name, "url": o.Url},
				}
			}
		case *message.ShortVideoElement:
			if ur {
				m = MSG{
					"type": "video",
					"data": map[string]string{"file": o.Name},
				}
			} else {
				m = MSG{
					"type": "video",
					"data": map[string]string{"file": o.Name, "url": o.Url},
				}
			}
		case *message.ImageElement:
			if ur {
				m = MSG{
					"type": "image",
					"data": map[string]string{"file": o.Filename},
				}
			} else {
				m = MSG{
					"type": "image",
					"data": map[string]string{"file": o.Filename, "url": o.Url},
				}
			}
		case *message.GroupImageElement:
			if ur {
				m = MSG{
					"type": "image",
					"data": map[string]string{"file": hex.EncodeToString(o.Md5) + ".image"},
				}
			} else {
				m = MSG{
					"type": "image",
					"data": map[string]string{"file": hex.EncodeToString(o.Md5) + ".image", "url": CQCodeEscapeText(o.Url)},
				}
			}
		case *message.FriendImageElement:
			if ur {
				m = MSG{
					"type": "image",
					"data": map[string]string{"file": hex.EncodeToString(o.Md5) + ".image"},
				}
			} else {
				m = MSG{
					"type": "image",
					"data": map[string]string{"file": hex.EncodeToString(o.Md5) + ".image", "url": CQCodeEscapeText(o.Url)},
				}
			}
		case *message.GroupFlashImgElement:
			return []MSG{{
				"type": "image",
				"data": map[string]string{"file": o.Filename, "type": "flash"},
			}}
		case *message.FriendFlashImgElement:
			return []MSG{{
				"type": "image",
				"data": map[string]string{"file": o.Filename, "type": "flash"},
			}}
		case *message.ServiceElement:
			if isOk := strings.Contains(o.Content, "<?xml"); isOk {
				m = MSG{
					"type": "xml",
					"data": map[string]string{"data": o.Content, "resid": fmt.Sprintf("%d", o.Id)},
				}
			} else {
				m = MSG{
					"type": "json",
					"data": map[string]string{"data": o.Content, "resid": fmt.Sprintf("%d", o.Id)},
				}
			}
		default:
			continue
		}
		r = append(r, m)
	}
	return
}

// ToStringMessage 将消息元素数组转为字符串以用于消息上报
func ToStringMessage(e []message.IMessageElement, id int64, isRaw ...bool) (r string) {
	ur := false
	if len(isRaw) != 0 {
		ur = isRaw[0]
	}
	// 方便
	m := &message.SendingMessage{Elements: e}
	reply := m.FirstOrNil(func(e message.IMessageElement) bool {
		_, ok := e.(*message.ReplyElement)
		return ok
	})
	if reply != nil {
		r += fmt.Sprintf("[CQ:reply,id=%d]", toGlobalID(id, reply.(*message.ReplyElement).ReplySeq))
	}
	for _, elem := range e {
		switch o := elem.(type) {
		case *message.TextElement:
			r += CQCodeEscapeText(o.Content)
		case *message.AtElement:
			if o.Target == 0 {
				r += "[CQ:at,qq=all]"
				continue
			}
			r += fmt.Sprintf("[CQ:at,qq=%d]", o.Target)
		case *message.RedBagElement:
			r += fmt.Sprintf("[CQ:redbag,title=%s]", o.Title)
		case *message.ForwardElement:
			r += fmt.Sprintf("[CQ:forward,id=%s]", o.ResId)
		case *message.FaceElement:
			r += fmt.Sprintf(`[CQ:face,id=%d]`, o.Index)
		case *message.VoiceElement:
			if ur {
				r += fmt.Sprintf(`[CQ:record,file=%s]`, o.Name)
			} else {
				r += fmt.Sprintf(`[CQ:record,file=%s,url=%s]`, o.Name, CQCodeEscapeValue(o.Url))
			}
		case *message.ShortVideoElement:
			if ur {
				r += fmt.Sprintf(`[CQ:video,file=%s]`, o.Name)
			} else {
				r += fmt.Sprintf(`[CQ:video,file=%s,url=%s]`, o.Name, CQCodeEscapeValue(o.Url))
			}
		case *message.ImageElement:
			if ur {
				r += fmt.Sprintf(`[CQ:image,file=%s]`, o.Filename)
			} else {
				r += fmt.Sprintf(`[CQ:image,file=%s,url=%s]`, o.Filename, CQCodeEscapeValue(o.Url))
			}
		case *message.GroupImageElement:
			if ur {
				r += fmt.Sprintf("[CQ:image,file=%s]", hex.EncodeToString(o.Md5)+".image")
			} else {
				r += fmt.Sprintf("[CQ:image,file=%s,url=%s]", hex.EncodeToString(o.Md5)+".image", CQCodeEscapeText(o.Url))
			}
		case *message.FriendImageElement:
			if ur {
				r += fmt.Sprintf("[CQ:image,file=%s]", hex.EncodeToString(o.Md5)+".image")
			} else {
				r += fmt.Sprintf("[CQ:image,file=%s,url=%s]", hex.EncodeToString(o.Md5)+".image", CQCodeEscapeText(o.Url))
			}
		case *message.GroupFlashImgElement:
			return fmt.Sprintf("[CQ:image,type=flash,file=%s]", o.Filename)
		case *message.FriendFlashImgElement:
			return fmt.Sprintf("[CQ:image,type=flash,file=%s]", o.Filename)
		case *message.ServiceElement:
			if isOk := strings.Contains(o.Content, "<?xml"); isOk {
				r += fmt.Sprintf(`[CQ:xml,data=%s,resid=%d]`, CQCodeEscapeValue(o.Content), o.Id)
			} else {
				r += fmt.Sprintf(`[CQ:json,data=%s,resid=%d]`, CQCodeEscapeValue(o.Content), o.Id)
			}
		case *message.LightAppElement:
			r += fmt.Sprintf(`[CQ:json,data=%s]`, CQCodeEscapeValue(o.Content))
			// r += CQCodeEscapeText(o.Content)
		}
	}
	return
}

// ConvertStringMessage 将消息字符串转为消息元素数组
func (bot *CQBot) ConvertStringMessage(s string, isGroup bool) (r []message.IMessageElement) {
	var t, key string
	var d = map[string]string{}
	ptr := unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data)
	l := len(s)
	i, j, CQBegin := 0, 0, 0

	saveCQCode := func() {
		if t == "reply" { // reply 特殊处理
			if len(r) > 0 {
				if _, ok := r[0].(*message.ReplyElement); ok {
					log.Warnf("警告: 一条信息只能包含一个 Reply 元素.")
					return
				}
			}
			customText := d["text"]
			sender, err := strconv.ParseInt(d["qq"], 10, 64)
			if err != nil {
				log.Warnf("警告:自定义 Reply 元素中必须包含 Uin")
				return
			}
			msgTime, err := strconv.ParseInt(d["time"], 10, 64)
			if err != nil {
				msgTime = time.Now().Unix()
			}
			messageSeq, err := strconv.ParseInt(d["seq"], 10, 64)
			if err != nil {
				messageSeq = 0
			}
			r = append([]message.IMessageElement{
				&message.ReplyElement{
					ReplySeq: int32(messageSeq),
					Sender:   sender,
					Time:     int32(msgTime),
					Elements: bot.ConvertStringMessage(customText, isGroup),
				},
			}, r...)
			return
		}
		if t == "forward" { // 单独处理转发
			if id, ok := d["id"]; ok {
				r = []message.IMessageElement{bot.Client.DownloadForwardMessage(id)}
				return
			}
		}
		elem, err := bot.ToElement(t, d)
		if err != nil {
			org := s[CQBegin:i]
			if !IgnoreInvalidCQCode {
				log.Warnf("转换CQ码 %v 时出现错误: %v 将原样发送.", org, err)
				r = append(r, message.NewText(org))
			} else {
				log.Warnf("转换CQ码 %v 时出现错误: %v 将忽略.", org, err)
			}
			return
		}
		switch i := elem.(type) {
		case message.IMessageElement:
			r = append(r, i)
		case []message.IMessageElement:
			r = append(r, i...)
		}
	}

S1: // Plain Text
	for ; i < l; i++ {
		if *(*byte)(add(ptr, uintptr(i))) == '[' && i+4 < l &&
			*(*uint32)(add(ptr, uintptr(i))) == magicCQ { // Magic :uint32([]byte("[CQ:"))
			if i > j {
				r = append(r, message.NewText(CQCodeUnescapeText(s[j:i])))
			}
			CQBegin = i
			i += 4
			j = i
			goto S2
		}
	}
	goto End
S2: // CQCode Type
	for k := range d { // 内存复用，减小GC压力
		delete(d, k)
	}
	for ; i < l; i++ {
		switch *(*byte)(add(ptr, uintptr(i))) {
		case ',': // CQ Code with params
			t = s[j:i]
			i++
			j = i
			goto S3
		case ']': // CQ Code without params
			t = s[j:i]
			i++
			j = i
			saveCQCode()
			goto S1
		}
	}
	goto End
S3: // CQCode param key
	for ; i < l; i++ {
		if *(*byte)(add(ptr, uintptr(i))) == '=' {
			key = s[j:i]
			i++
			j = i
			goto S4
		}
	}
	goto End
S4: // CQCode param value
	for ; i < l; i++ {
		switch *(*byte)(add(ptr, uintptr(i))) {
		case ',': // more param
			d[key] = CQCodeUnescapeValue(s[j:i])
			i++
			j = i
			goto S3
		case ']':
			d[key] = CQCodeUnescapeValue(s[j:i])
			i++
			j = i
			saveCQCode()
			goto S1
		}
	}
	goto End
End:
	if i > j {
		r = append(r, message.NewText(CQCodeUnescapeText(s[j:i])))
	}
	return
}

// ConvertObjectMessage 将消息JSON对象转为消息元素数组
func (bot *CQBot) ConvertObjectMessage(m gjson.Result, isGroup bool) (r []message.IMessageElement) {
	convertElem := func(e gjson.Result) {
		t := e.Get("type").Str
		if t == "reply" && isGroup {
			if len(r) > 0 {
				if _, ok := r[0].(*message.ReplyElement); ok {
					log.Warnf("警告: 一条信息只能包含一个 Reply 元素.")
					return
				}
			}
			customText := e.Get("data").Get("text").String()
			sender, err := strconv.ParseInt(e.Get("data").Get("qq").String(), 10, 64)
			if err != nil {
				log.Warnf("警告:自定义 Reply 元素中必须包含 Uin")
				return
			}
			msgTime, err := strconv.ParseInt(e.Get("data").Get("time").String(), 10, 64)
			if err != nil {
				msgTime = time.Now().Unix()
			}
			messageSeq, err := strconv.ParseInt(e.Get("data").Get("seq").String(), 10, 64)
			if err != nil {
				messageSeq = 0
			}
			r = append([]message.IMessageElement{
				&message.ReplyElement{
					ReplySeq: int32(messageSeq),
					Sender:   sender,
					Time:     int32(msgTime),
					Elements: bot.ConvertStringMessage(customText, isGroup),
				},
			}, r...)
			return
		}
		if t == "forward" {
			r = []message.IMessageElement{bot.Client.DownloadForwardMessage(e.Get("data.id").String())}
			return
		}
		d := make(map[string]string)
		e.Get("data").ForEach(func(key, value gjson.Result) bool {
			d[key.Str] = value.String()
			return true
		})
		elem, err := bot.ToElement(t, d)
		if err != nil {
			log.Warnf("转换CQ码 (%v) 到MiraiGo Element时出现错误: %v 将忽略本段CQ码.", e.Raw, err)
			return
		}
		switch i := elem.(type) {
		case message.IMessageElement:
			r = append(r, i)
		case []message.IMessageElement:
			r = append(r, i...)
		}
	}
	if m.Type == gjson.String {
		return bot.ConvertStringMessage(m.Str, isGroup)
	}
	if m.IsArray() {
		for _, e := range m.Array() {
			convertElem(e)
		}
	}
	if m.IsObject() {
		convertElem(m)
	}
	return
}

// ToElement 将解码后的CQCode转换为Element.
//
// 返回 interface{} 存在三种类型
//
// message.IMessageElement []message.IMessageElement nil
func (bot *CQBot) ToElement(t string, d map[string]string) (m interface{}, err error) {
	switch t {
	case "text":
		if SplitURL {
			var ret []message.IMessageElement
			for _, text := range global.SplitURL(d["text"]) {
				ret = append(ret, message.NewText(text))
			}
			return ret, nil
		}
		return message.NewText(d["text"]), nil
	case "video":
		cache := d["cache"]
		if cache == "" {
			cache = "1"
		}
		file, err := bot.makeImageOrVideoElem(d, true)
		if err != nil {
			return nil, err
		}
		v := file.(*LocalVideoElement)
		if v.File == "" {
			return v, nil
		}
		var data []byte
		_ = global.ExtractCover(v.File, v.File+".jpg")
		data, _ = ioutil.ReadFile(v.File + ".jpg")
		v.thumb = bytes.NewReader(data)
		video, _ := os.Open(v.File)
		defer video.Close()
		_, err = video.Seek(4, io.SeekStart)
		if err != nil {
			return nil, err
		}
		var header = make([]byte, 4)
		_, err = video.Read(header)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(header, []byte{0x66, 0x74, 0x79, 0x70}) { // check file header ftyp
			_, _ = video.Seek(0, io.SeekStart)
			hash, _ := utils.ComputeMd5AndLength(video)
			cacheFile := path.Join(global.CachePath, hex.EncodeToString(hash[:])+".mp4")
			if global.PathExists(cacheFile) && cache == "1" {
				goto ok
			}
			err = global.EncodeMP4(v.File, cacheFile)
			if err != nil {
				return nil, err
			}
		ok:
			v.File = cacheFile
		}
		return v, nil
	default:
		return nil, errors.New("unsupported cq code: " + t)
	}
}

/*CQCodeEscapeText 将字符串raw中部分字符转义

& -> &amp;

[ -> &#91;

] -> &#93;

*/
func CQCodeEscapeText(raw string) string {
	ret := raw
	ret = strings.ReplaceAll(ret, "&", "&amp;")
	ret = strings.ReplaceAll(ret, "[", "&#91;")
	ret = strings.ReplaceAll(ret, "]", "&#93;")
	return ret
}

/*CQCodeEscapeValue 将字符串value中部分字符转义

, -> &#44;

& -> &amp;

[ -> &#91;

] -> &#93;

*/
func CQCodeEscapeValue(value string) string {
	ret := CQCodeEscapeText(value)
	ret = strings.ReplaceAll(ret, ",", "&#44;")
	return ret
}

/*CQCodeUnescapeText 将字符串content中部分字符反转义

&amp; -> &

&#91; -> [

&#93; -> ]

*/
func CQCodeUnescapeText(content string) string {
	ret := content
	ret = strings.ReplaceAll(ret, "&#91;", "[")
	ret = strings.ReplaceAll(ret, "&#93;", "]")
	ret = strings.ReplaceAll(ret, "&amp;", "&")
	return ret
}

/*CQCodeUnescapeValue 将字符串content中部分字符反转义

&#44; -> ,

&amp; -> &

&#91; -> [

&#93; -> ]

*/
func CQCodeUnescapeValue(content string) string {
	ret := strings.ReplaceAll(content, "&#44;", ",")
	ret = CQCodeUnescapeText(ret)
	return ret
}

// makeImageOrVideoElem 图片 elem 生成器，单独拎出来，用于公用
func (bot *CQBot) makeImageOrVideoElem(d map[string]string, video bool) (message.IMessageElement, error) {
	f := d["file"]
	if strings.HasPrefix(f, "http") || strings.HasPrefix(f, "https") {
		cache := d["cache"]
		c := d["c"]
		if cache == "" {
			cache = "1"
		}
		hash := md5.Sum([]byte(f))
		cacheFile := path.Join(global.CachePath, hex.EncodeToString(hash[:])+".cache")
		thread, _ := strconv.Atoi(c)
		if global.PathExists(cacheFile) && cache == "1" {
			goto hasCacheFile
		}
		if global.PathExists(cacheFile) {
			_ = os.Remove(cacheFile)
		}
		if err := global.DownloadFileMultiThreading(f, cacheFile, maxVideoSize, thread, nil); err != nil {
			return nil, err
		}
	hasCacheFile:
		if video {
			return &LocalVideoElement{File: cacheFile}, nil
		}
	}
	if strings.HasPrefix(f, "file") {
		fu, err := url.Parse(f)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(fu.Path, "/") && runtime.GOOS == `windows` {
			fu.Path = fu.Path[1:]
		}
		info, err := os.Stat(fu.Path)
		if err != nil {
			if !os.IsExist(err) {
				return nil, errors.New("file not found")
			}
			return nil, err
		}
		if video {
			if info.Size() == 0 || info.Size() >= maxVideoSize {
				return nil, errors.New("invalid video size")
			}
			return &LocalVideoElement{File: fu.Path}, nil
		}
	}
	rawPath := path.Join(global.VideoPath, f)
	if !global.PathExists(rawPath) {
		return nil, errors.New("invalid video")
	}
	if path.Ext(rawPath) == ".video" {
		b, _ := ioutil.ReadFile(rawPath)
		r := binary.NewReader(b)
		return &LocalVideoElement{ShortVideoElement: message.ShortVideoElement{ // todo 检查缓存是否有效
			Md5:       r.ReadBytes(16),
			ThumbMd5:  r.ReadBytes(16),
			Size:      r.ReadInt32(),
			ThumbSize: r.ReadInt32(),
			Name:      r.ReadString(),
			Uuid:      r.ReadAvailable(),
		}}, nil
	}
	return &LocalVideoElement{File: rawPath}, nil
}
