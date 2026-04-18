package douyincrawler

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"sync/atomic"

	pb "ks-prank/pkg/douyincrawler/proto"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// 定义各种消息处理器接口
type GiftHandler interface {
	HandleGift(user *pb.User, giftInfo *pb.GiftMessage)
}

type ChatHandler interface {
	HandleChat(user *pb.User, chatInfo *pb.ChatMessage)
}

type MemberHandler interface {
	HandleMember(user *pb.User, memberInfo *pb.MemberMessage)
}

type LikeHandler interface {
	HandleLike(user *pb.User, likeInfo *pb.LikeMessage)
}

// 消息处理器配置
type MessageHandlerConfig struct {
	GiftHandler   GiftHandler
	ChatHandler   ChatHandler
	MemberHandler MemberHandler
	LikeHandler   LikeHandler
}

type MessageHandler interface {
	ProcessMessage(data []byte, client *DouyinClient) error
}

type DefaultMessageHandler struct {
	config   MessageHandlerConfig
	ackCount uint64
}

const ackLogSampleEvery uint64 = 100

func NewMessageHandler(config MessageHandlerConfig) MessageHandler {
	return &DefaultMessageHandler{
		config: config,
	}
}

func (h *DefaultMessageHandler) ProcessMessage(data []byte, client *DouyinClient) error {
	// 1. 解析外层 PushFrame
	frame := &pb.PushFrame{}
	if err := proto.Unmarshal(data, frame); err != nil {
		return err
	}

	// 2. 解压 payload
	decompressed, err := decompress(frame.Payload)
	if err != nil {
		return err
	}

	// 3. 解析 Response
	response := &pb.Response{}
	if err := proto.Unmarshal(decompressed, response); err != nil {
		return err
	}
	client.UpdateHeartbeatDuration(response.GetHeartbeatDuration())

	// 4. 处理每条消息
	for _, msg := range response.MessagesList {
		if err := h.handleMessage(msg); err != nil {
			log.Printf("处理消息失败: %v", err)
		}
	}

	// 5. 如果需要 ACK，发送确认
	if response.NeedAck {
		if err := h.sendAck(frame.LogId, response.InternalExt, client); err != nil {
			log.Printf("ACK发送失败，logId=%d err=%v", frame.LogId, err)
			return err
		}
		total := atomic.AddUint64(&h.ackCount, 1)
		if total == 1 || total%ackLogSampleEvery == 0 {
			log.Printf("ACK采样日志: total=%d latestLogId=%d", total, frame.LogId)
		}
	}

	return nil
}

func (h *DefaultMessageHandler) handleMessage(msg *pb.Message) error {
	switch msg.Method {
	case "WebcastChatMessage":
		return h.handleChatMessage(msg)
	case "WebcastGiftMessage":
		return h.handleGiftMessage(msg)
	case "WebcastMemberMessage":
		return h.handleMemberMessage(msg)
	case "WebcastLikeMessage":
		return h.handleLikeMessage(msg)
	default:
		// log.Printf("未处理的消息类型: %s", msg.Method)
		return nil
	}
}

func (h *DefaultMessageHandler) handleChatMessage(msg *pb.Message) error {
	chatMsg := &pb.ChatMessage{}
	if err := proto.Unmarshal(msg.Payload, chatMsg); err != nil {
		return err
	}

	if h.config.ChatHandler != nil && chatMsg.User != nil {
		h.config.ChatHandler.HandleChat(chatMsg.User, chatMsg)
	} else {
		if chatMsg.User != nil {
			log.Printf("[聊天] [%s] %s", chatMsg.User.NickName, chatMsg.Content)
			if avatar := chatMsg.User.AvatarThumb; avatar != nil {
				log.Printf("用户头像: %v", avatar.GetUrlListList())
			}
		}
	}
	return nil
}

func (h *DefaultMessageHandler) handleGiftMessage(msg *pb.Message) error {
	giftMsg := &pb.GiftMessage{}
	if err := proto.Unmarshal(msg.Payload, giftMsg); err != nil {
		return err
	}

	if h.config.GiftHandler != nil && giftMsg.User != nil {
		h.config.GiftHandler.HandleGift(giftMsg.User, giftMsg)
	} else {
		if giftMsg.User != nil {
			log.Printf("[礼物] %s", giftMsg.Common.Describe)
			if avatar := giftMsg.User.AvatarThumb; avatar != nil {
				log.Printf("送礼用户头像: %v", avatar.GetUrlListList())
			}
		}
	}
	return nil
}

func (h *DefaultMessageHandler) handleMemberMessage(msg *pb.Message) error {
	memberMsg := &pb.MemberMessage{}
	if err := proto.Unmarshal(msg.Payload, memberMsg); err != nil {
		return err
	}

	if h.config.MemberHandler != nil && memberMsg.User != nil {
		h.config.MemberHandler.HandleMember(memberMsg.User, memberMsg)
	} else {
		if memberMsg.User != nil {
			log.Printf("[进入直播间] %s", memberMsg.User.NickName)
			if avatar := memberMsg.User.AvatarThumb; avatar != nil {
				log.Printf("用户头像: %v", avatar.GetUrlListList())
			}
		}
	}
	return nil
}

func (h *DefaultMessageHandler) handleLikeMessage(msg *pb.Message) error {
	likeMsg := &pb.LikeMessage{}
	if err := proto.Unmarshal(msg.Payload, likeMsg); err != nil {
		return err
	}

	if h.config.LikeHandler != nil && likeMsg.User != nil {
		h.config.LikeHandler.HandleLike(likeMsg.User, likeMsg)
	}
	return nil
}

func (h *DefaultMessageHandler) sendAck(logId uint64, internalExt string, client *DouyinClient) error {
	ack := &pb.PushFrame{
		PayloadType: "ack",
		LogId:       logId,
		Payload:     []byte(internalExt),
	}

	data, err := proto.Marshal(ack)
	if err != nil {
		return err
	}

	return client.SendMessage(websocket.BinaryMessage, data)
}

// 工具函数
func decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
