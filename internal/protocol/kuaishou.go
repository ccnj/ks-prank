package protocol

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math/rand"
	"time"

	pb "ks-prank/proto"

	"google.golang.org/protobuf/proto"
)

func GeneratePageID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%s_%d", string(b), time.Now().UnixMilli())
}

func BuildEnterRoomMsg(token, liveStreamID string) ([]byte, error) {
	enterRoom := &pb.CSWebEnterRoom{
		Token:        proto.String(token),
		LiveStreamId: proto.String(liveStreamID),
		PageId:       proto.String(GeneratePageID()),
	}
	enterPayload, err := proto.Marshal(enterRoom)
	if err != nil {
		return nil, err
	}

	msg := &pb.SocketMessage{
		PayloadType:     pb.PayloadType_CS_ENTER_ROOM.Enum(),
		CompressionType: pb.SocketMessage_NONE.Enum(),
		Payload:         enterPayload,
	}
	return proto.Marshal(msg)
}

func BuildHeartbeatMsg() ([]byte, error) {
	hb := &pb.CSWebHeartbeat{
		Timestamp: proto.Uint64(uint64(time.Now().UnixMilli())),
	}
	hbPayload, err := proto.Marshal(hb)
	if err != nil {
		return nil, err
	}

	msg := &pb.SocketMessage{
		PayloadType:     pb.PayloadType_CS_HEARTBEAT.Enum(),
		CompressionType: pb.SocketMessage_NONE.Enum(),
		Payload:         hbPayload,
	}
	return proto.Marshal(msg)
}

func DecompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// ParseSocketMessage 解析外层 SocketMessage，处理 GZIP 解压，返回 payloadType 和解压后的 payload
func ParseSocketMessage(data []byte) (pb.PayloadType, []byte, error) {
	msg := &pb.SocketMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return 0, nil, fmt.Errorf("解析 SocketMessage 失败: %w", err)
	}

	payload := msg.Payload
	if msg.CompressionType != nil && *msg.CompressionType == pb.SocketMessage_GZIP {
		var err error
		payload, err = DecompressGzip(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("GZIP 解压失败: %w", err)
		}
	}

	return msg.GetPayloadType(), payload, nil
}
