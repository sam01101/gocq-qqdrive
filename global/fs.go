package global

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kardianos/osext"

	"github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
)

const (
	// VideoPath go-cqhttp使用的视频缓存目录
	VideoPath = "data/videos"
	// CachePath go-cqhttp使用的缓存目录
	CachePath = "data/cache"
)

// PathExists 判断给定path是否存在
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

// ReadAllText 读取给定path对应文件，无法读取时返回空值
func ReadAllText(path string) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		log.Error(err)
		return ""
	}
	return string(b)
}

// WriteAllText 将给定text写入给定path
func WriteAllText(path, text string) error {
	return ioutil.WriteFile(path, []byte(text), 0644)
}

// DelFile 删除一个给定path，并返回删除结果
func DelFile(path string) bool {
	err := os.Remove(path)
	if err != nil {
		// 删除失败
		log.Error(err)
		return false
	}
	// 删除成功
	log.Info(path + "删除成功")
	return true
}

// ReadAddrFile 从给定path中读取合法的IP地址与端口,每个IP地址以换行符"\n"作为分隔
func ReadAddrFile(path string) []*net.TCPAddr {
	d, err := ioutil.ReadFile(path)
	if err != nil {
		return nil
	}
	str := string(d)
	lines := strings.Split(str, "\n")
	var ret []*net.TCPAddr
	for _, l := range lines {
		ip := strings.Split(strings.TrimSpace(l), ":")
		if len(ip) == 2 {
			port, _ := strconv.Atoi(ip[1])
			ret = append(ret, &net.TCPAddr{IP: net.ParseIP(ip[0]), Port: port})
		}
	}
	return ret
}

// WriteCounter 写入量计算实例
type WriteCounter struct {
	Total uint64
}

// Write 方法将写入的byte长度追加至写入的总长度Total中
func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

// PrintProgress 方法将打印当前的总写入量
func (wc *WriteCounter) PrintProgress() {
	fmt.Printf("\r%s", strings.Repeat(" ", 35))
	fmt.Printf("\rDownloading... %s complete", humanize.Bytes(wc.Total))
}

// UpdateFromStream copy form getlantern/go-update
func UpdateFromStream(updateWith io.Reader) (err error, errRecover error) {
	updatePath, err := osext.Executable()
	if err != nil {
		return
	}
	var newBytes []byte
	// no patch to apply, go on through
	var fileHeader []byte
	bufBytes := bufio.NewReader(updateWith)
	fileHeader, err = bufBytes.Peek(2)
	if err != nil {
		return
	}
	// The content is always bzip2 compressed except when running test, in
	// which case is not prefixed with the magic byte sequence for sure.
	if bytes.Equal([]byte{0x42, 0x5a}, fileHeader) {
		// Identifying bzip2 files.
		updateWith = bzip2.NewReader(bufBytes)
	} else {
		updateWith = io.Reader(bufBytes)
	}
	newBytes, err = ioutil.ReadAll(updateWith)
	if err != nil {
		return
	}
	// get the directory the executable exists in
	updateDir := filepath.Dir(updatePath)
	filename := filepath.Base(updatePath)
	// Copy the contents of of newbinary to a the new executable file
	newPath := filepath.Join(updateDir, fmt.Sprintf(".%s.new", filename))
	fp, err := os.OpenFile(newPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return
	}
	// We won't log this error, because it's always going to happen.
	defer func() { _ = fp.Close() }()
	if _, err = io.Copy(fp, bytes.NewReader(newBytes)); err != nil {
		log.Errorf("Unable to copy data: %v\n", err)
	}

	// if we don't call fp.Close(), windows won't let us move the new executable
	// because the file will still be "in use"
	if err := fp.Close(); err != nil {
		log.Errorf("Unable to close file: %v\n", err)
	}
	// this is where we'll move the executable to so that we can swap in the updated replacement
	oldPath := filepath.Join(updateDir, fmt.Sprintf(".%s.old", filename))

	// delete any existing old exec file - this is necessary on Windows for two reasons:
	// 1. after a successful update, Windows can't remove the .old file because the process is still running
	// 2. windows rename operations fail if the destination file already exists
	_ = os.Remove(oldPath)

	// move the existing executable to a new file in the same directory
	err = os.Rename(updatePath, oldPath)
	if err != nil {
		return
	}

	// move the new executable in to become the new program
	err = os.Rename(newPath, updatePath)

	if err != nil {
		// copy unsuccessful
		errRecover = os.Rename(oldPath, updatePath)
	} else {
		// copy successful, remove the old binary
		_ = os.Remove(oldPath)
	}
	return
}
