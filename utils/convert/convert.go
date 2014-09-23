package convert

import (
	"crypto/md5"
	"fmt"
	"io"
	"time"
)

// 均采用大端字节序，事实上这个字节序没有任何用处，
// 只要服务器和客户端约定采用相同的字节序就行

func Uint32ToBytes(v uint32) []byte {
	buf := make([]byte, 4)
	buf[0] = byte(v >> 24)
	buf[1] = byte(v >> 16)
	buf[2] = byte(v >> 8)
	buf[3] = byte(v)
	return buf
}

func Int32ToBytes(v int32) []byte {
	buf := make([]byte, 4)
	buf[0] = byte(v >> 24)
	buf[1] = byte(v >> 16)
	buf[2] = byte(v >> 8)
	buf[3] = byte(v)
	return buf
}

func Uint16ToBytes(v uint16) []byte {
	buf := make([]byte, 2)
	buf[0] = byte(v >> 8)
	buf[1] = byte(v)
	return buf
}

func Int16ToBytes(v int16) []byte {
	buf := make([]byte, 2)
	buf[0] = byte(v >> 8)
	buf[1] = byte(v)
	return buf
}

func BytesToUint32(buf []byte) uint32 {
	v := (uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3]))
	return v
}

func BytesToInt32(buf []byte) int32 {
	v := (int32(buf[0])<<24 | int32(buf[1])<<16 | int32(buf[2])<<8 | int32(buf[3]))
	return v
}

func BytesToUint16(buf []byte) uint16 {
	v := (uint16(buf[0])<<8 | uint16(buf[1]))
	return v
}

func BytesToInt16(buf []byte) int16 {
	v := (int16(buf[0])<<8 | int16(buf[1]))
	return v
}

func TimestampToTimeString(timestamp int64) string {
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
}

func TimestampToTime(timestamp int64) time.Time {
	return time.Unix(timestamp, 0)
}

// 计算string的md5值， 返回长度32的字符串(md5)
func StringToMd5(s string) string {
	m := md5.New()
	io.WriteString(m, s)
	return fmt.Sprintf("%x", m.Sum(nil))
}
