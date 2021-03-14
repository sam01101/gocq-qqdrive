package global

import (
	"github.com/pkg/errors"
	"os/exec"
)

// EncodeMP4 将给定视频文件编码为MP4
func EncodeMP4(src string, dst string) error { //        -y 覆盖文件
	cmd1 := exec.Command("ffmpeg", "-i", src, "-y", "-c", "copy", "-map", "0", dst)
	err := cmd1.Run()
	if err != nil {
		cmd2 := exec.Command("ffmpeg", "-i", src, "-y", "-c:v", "h264", "-c:a", "mp3", dst)
		return errors.Wrap(cmd2.Run(), "convert mp4 failed")
	}
	return err
}

// ExtractCover 获取给定视频文件的Cover
func ExtractCover(src string, target string) error {
	cmd := exec.Command("ffmpeg", "-i", src, "-y", "-r", "1", "-f", "image2", target)
	return errors.Wrap(cmd.Run(), "extract video cover failed")
}
