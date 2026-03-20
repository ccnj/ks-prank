package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	pb "ks-prank/proto"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type giftInfo struct {
	Name  string
	Price int // 快币单价
}

// 快手礼物ID -> 信息映射（从 /live_api/liveroom/giftlist 接口获取）
var giftMap = map[uint32]giftInfo{
	9: {Name: "啤酒", Price: 10},
	14: {Name: "钻戒", Price: 66},
	16: {Name: "皇冠", Price: 188},
	113: {Name: "火箭", Price: 328},
	114: {Name: "玫瑰", Price: 1},
	165: {Name: "小可爱", Price: 1},
	197: {Name: "棒棒糖", Price: 1},
	219: {Name: "告白气球", Price: 208},
	224: {Name: "跑车", Price: 666},
	225: {Name: "穿云箭", Price: 2888},
	269: {Name: "游乐园", Price: 6666},
	275: {Name: "喜欢你", Price: 298},
	287: {Name: "真爱大炮", Price: 1314},
	289: {Name: "玫瑰花园", Price: 520},
	297: {Name: "超跑车队", Price: 1888},
	306: {Name: "浪漫游轮", Price: 2288},
	311: {Name: "私人飞机", Price: 999},
	322: {Name: "比心", Price: 99},
	327: {Name: "恋爱宇宙", Price: 13140},
	336: {Name: "天鹅湖", Price: 666},
	342: {Name: "银河之恋", Price: 1999},
	10006: {Name: "人气卡", Price: 50},
	10012: {Name: "终于等到你", Price: 28888},
	10116: {Name: "月球之旅", Price: 2888},
	10194: {Name: "童话日记", Price: 188},
	10342: {Name: "浪漫花车", Price: 399},
	10381: {Name: "小白菜", Price: 1},
	10385: {Name: "私人海岛", Price: 1666},
	10415: {Name: "娶你进门", Price: 520},
	10470: {Name: "YYDS", Price: 188},
	10482: {Name: "擂鼓助威", Price: 10},
	10509: {Name: "千军万马", Price: 25000},
	10657: {Name: "送你花环", Price: 599},
	10679: {Name: "旋转木马", Price: 520},
	10815: {Name: "遨游太空", Price: 999},
	10816: {Name: "跨时空之恋", Price: 9999},
	11020: {Name: "超神", Price: 99},
	11021: {Name: "电玩耳机", Price: 10},
	11022: {Name: "欧气手柄", Price: 66},
	11070: {Name: "为你盖楼", Price: 2999},
	11213: {Name: "做我的新娘", Price: 520},
	11257: {Name: "守护流星雨", Price: 2999},
	11282: {Name: "海底宫殿", Price: 2999},
	11283: {Name: "美人鱼之恋", Price: 19999},
	11477: {Name: "机甲摩托", Price: 666},
	11478: {Name: "悬浮飞车", Price: 888},
	11479: {Name: "未来航母", Price: 1888},
	11480: {Name: "星际战舰", Price: 18888},
	11518: {Name: "思念千纸鹤", Price: 10},
	11537: {Name: "快乐星球", Price: 30000},
	11783: {Name: "摸摸头", Price: 99},
	11784: {Name: "捏捏脸", Price: 99},
	11825: {Name: "转角遇到爱", Price: 299},
	11828: {Name: "浪漫童话城", Price: 5200},
	11886: {Name: "显眼包", Price: 288},
	11967: {Name: "时空唤醒", Price: 2999},
	11968: {Name: "誓死守护", Price: 19999},
	12014: {Name: "夏日小酒馆", Price: 188},
	12115: {Name: "胜利之水", Price: 66},
	12117: {Name: "荣耀王冠", Price: 288},
	12169: {Name: "全军出击", Price: 2888},
	12188: {Name: "真心话大挑战", Price: 299},
	12624: {Name: "为你簪花", Price: 520},
	12765: {Name: "蛋", Price: 10},
	12826: {Name: "黑子说话", Price: 1},
	12827: {Name: "笑死", Price: 1},
	12831: {Name: "蓝色妖姬", Price: 52},
	12833: {Name: "要爱了", Price: 126},
	12834: {Name: "流星花园", Price: 1200},
	12836: {Name: "烛光晚餐", Price: 266},
	12837: {Name: "请你看电影", Price: 921},
	12838: {Name: "浪漫之约", Price: 4999},
	12840: {Name: "心心点灯", Price: 150},
	12841: {Name: "为爱干杯", Price: 258},
	12842: {Name: "月光旅行", Price: 728},
	12843: {Name: "相约樱花林", Price: 1999},
	12844: {Name: "仙子下凡", Price: 3000},
	12845: {Name: "相思寄明月", Price: 3999},
	12846: {Name: "思念到永久", Price: 4499},
	12847: {Name: "荷花泛舟", Price: 6000},
	12848: {Name: "浪漫土耳其", Price: 799},
	12849: {Name: "求婚套装", Price: 9000},
	12851: {Name: "决胜时刻", Price: 25000},
	12855: {Name: "一路有你", Price: 1688},
	12856: {Name: "电玩女神", Price: 28888},
	12857: {Name: "鲸鱼骑士", Price: 11999},
	12858: {Name: "倾城之吻", Price: 29999},
	12859: {Name: "热浪舞池", Price: 7500},
	12860: {Name: "心动眼镜", Price: 499},
	12861: {Name: "一定发财", Price: 88},
	12862: {Name: "姻缘树", Price: 1000},
	12863: {Name: "浪漫营地", Price: 5000},
	12868: {Name: "甜蜜如你", Price: 126},
	12869: {Name: "墨镜", Price: 59},
	12870: {Name: "爱情广场", Price: 2100},
	12871: {Name: "金色婚礼", Price: 2599},
	12872: {Name: "金色美酒", Price: 419},
	12873: {Name: "抱抱熊", Price: 588},
	12874: {Name: "深海鲸语", Price: 758},
	12875: {Name: "雷霆战机", Price: 10000},
	12876: {Name: "胜利握手", Price: 126},
	12877: {Name: "芳心猎手", Price: 4321},
	12878: {Name: "专属浪漫", Price: 899},
	12882: {Name: "陪你看日出", Price: 726},
	12883: {Name: "三生三世", Price: 3344},
	12892: {Name: "水晶鞋", Price: 66},
	12895: {Name: "你真美", Price: 2},
	12896: {Name: "太6了", Price: 6},
	12897: {Name: "送你小花花", Price: 10},
	12904: {Name: "喝彩", Price: 1},
	12905: {Name: "情窦初开", Price: 2},
	12906: {Name: "香水", Price: 5},
	12907: {Name: "爱心抱枕", Price: 10},
	12908: {Name: "加油鸭", Price: 15},
	12909: {Name: "爱你鸭", Price: 20},
	12941: {Name: "大花头巾", Price: 199},
	12942: {Name: "捶捶肩", Price: 59},
	12944: {Name: "喝可乐", Price: 59},
	12945: {Name: "粉熊抱抱", Price: 199},
	12946: {Name: "冲澡鸭", Price: 298},
	12947: {Name: "玫瑰吻", Price: 99},
	12959: {Name: "赞", Price: 1},
	13004: {Name: "奶茶", Price: 10},
	13005: {Name: "浪漫城堡", Price: 5200},
	13105: {Name: "炸弹", Price: 1},
	13106: {Name: "意大利炮", Price: 366},
	13107: {Name: "轰炸机", Price: 666},
	13108: {Name: "航母舰队", Price: 6666},
	13135: {Name: "笑一个", Price: 99},
	13136: {Name: "蟹蟹头套", Price: 199},
	13137: {Name: "亲亲你", Price: 99},
	13138: {Name: "捧脸杀", Price: 99},
	13399: {Name: "燃烧瓶", Price: 10},
	13400: {Name: "电击枪", Price: 66},
	13401: {Name: "宇宙飞船", Price: 20000},
	14042: {Name: "烟花金龙", Price: 10208},
	14091: {Name: "喜欢你烟花", Price: 506},
	14092: {Name: "烟花穿云箭", Price: 3096},
	14093: {Name: "烟花跑车", Price: 874},
	14094: {Name: "烟花飞机", Price: 1207},
	14095: {Name: "鲜花和烟花", Price: 507},
	14096: {Name: "喜欢穿云箭", Price: 3186},
	14097: {Name: "喜欢你跑车", Price: 964},
	14098: {Name: "喜欢你飞机", Price: 1297},
	14101: {Name: "跑车穿云箭", Price: 3554},
	14102: {Name: "跑车和飞机", Price: 1665},
	14103: {Name: "鲜花跑车", Price: 965},
	14104: {Name: "鲜花飞机", Price: 1298},
	14105: {Name: "飞机穿云箭", Price: 3887},
	14106: {Name: "鲜花穿云箭", Price: 3187},
	14247: {Name: "AI魔法", Price: 1},
	14250: {Name: "喜欢你鲜花", Price: 299},
	41527: {Name: "金钻龙表", Price: 888},
	41528: {Name: "豪车飞钞", Price: 6488},
	41529: {Name: "泼天富贵", Price: 7488},
	41530: {Name: "财富之门", Price: 8888},
	41535: {Name: "黄金项链", Price: 299},
	41988: {Name: "小雪人", Price: 10},
	41991: {Name: "雪人小镇", Price: 1999},
	42074: {Name: "柯基玩雪", Price: 299},
	1053328: {Name: "紫藤相拥", Price: 1314},
	1078207: {Name: "盛典票", Price: 1},
	1082531: {Name: "两只蝴蝶", Price: 299},
	1085665: {Name: "为你加冕", Price: 1000},
	1086029: {Name: "集结号", Price: 10},
	1155616: {Name: "花海游船", Price: 5200},
}

func getGiftName(id uint32) string {
	if info, ok := giftMap[id]; ok {
		return info.Name
	}
	return fmt.Sprintf("未知礼物(%d)", id)
}

func getGiftPrice(id uint32) int {
	if info, ok := giftMap[id]; ok {
		return info.Price
	}
	return 0
}

func generatePageID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%s_%d", string(b), time.Now().UnixMilli())
}

func buildEnterRoomMsg(token, liveStreamID string) ([]byte, error) {
	enterRoom := &pb.CSWebEnterRoom{
		Token:        proto.String(token),
		LiveStreamId: proto.String(liveStreamID),
		PageId:       proto.String(generatePageID()),
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

func buildHeartbeatMsg() ([]byte, error) {
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

func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func handleSocketMessage(data []byte) {
	msg := &pb.SocketMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		log.Printf("解析 SocketMessage 失败: %v", err)
		return
	}

	payload := msg.Payload
	if msg.CompressionType != nil && *msg.CompressionType == pb.SocketMessage_GZIP {
		var err error
		payload, err = decompressGzip(payload)
		if err != nil {
			log.Printf("GZIP 解压失败: %v", err)
			return
		}
	}

	switch msg.GetPayloadType() {
	case pb.PayloadType_SC_ENTER_ROOM_ACK:
		ack := &pb.SCWebEnterRoomAck{}
		if err := proto.Unmarshal(payload, ack); err == nil {
			log.Printf("[进房确认] code=%d msg=%s", ack.GetCode(), ack.GetMsg())
		}

	case pb.PayloadType_SC_HEARTBEAT_ACK:
		// 心跳回复，静默处理

	case pb.PayloadType_SC_FEED_PUSH:
		feed := &pb.SCWebFeedPush{}
		if err := proto.Unmarshal(payload, feed); err != nil {
			log.Printf("解析 SCWebFeedPush 失败: %v", err)
			return
		}
		handleFeedPush(feed)

	default:
		// 340=观众列表, 682=未知, 510=系统通知 等，静默处理
	}
}

func handleFeedPush(feed *pb.SCWebFeedPush) {
	// 处理礼物
	for _, gift := range feed.GiftFeeds {
		user := gift.User
		userName := "未知用户"
		if user != nil {
			userName = user.GetUserName()
		}
		giftID := gift.GetGiftId()
		giftName := getGiftName(giftID)
		price := getGiftPrice(giftID)
		log.Printf("[礼物] %s 送出 %s (%d快币) x%d (giftId=%d, comboCount=%d)",
			userName, giftName, price, gift.GetBatchSize(), giftID, gift.GetComboCount())
	}

	// 处理弹幕
	for _, comment := range feed.CommentFeeds {
		user := comment.User
		userName := "未知用户"
		if user != nil {
			userName = user.GetUserName()
		}
		log.Printf("[弹幕] %s: %s", userName, comment.GetContent())
	}

	// 处理点赞（量大，只在有礼物/弹幕时顺带打印）
	if len(feed.LikeFeeds) > 0 && (len(feed.GiftFeeds) > 0 || len(feed.CommentFeeds) > 0) {
		log.Printf("[点赞] 本次推送包含 %d 条点赞", len(feed.LikeFeeds))
	}
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Demo 模式：直接硬编码连接信息，验证协议是否能跑通
	wssURL := "wss://livejs-ws-group5.gifshow.com/websocket"
	token := "Hxfj8WpSi0ay9wmnSTtJBWBO5DOHLZ0twR4dKYbxd+GFFnM3r1fFubfAsRk1fJM2sqRfrQuRM0C86rzGUfWW7G2xWifTHM40qPSwlzPZMJMGtZYdFHSIbYuKptIIBFulAi/hdlQH0siF2/VBQ0VznnvCHIozIsBly2ZBMB93Y6RgViHe/XdyhYLrO8VG9JMeVL4bE5p2Y5BqY62whzKU3mlmj7tGA9xWVX7yCC7u1eY="
	liveStreamID := "nWRydQ_je14"

	// 允许通过命令行覆盖
	if len(os.Args) >= 4 {
		wssURL = os.Args[1]
		token = os.Args[2]
		liveStreamID = os.Args[3]
	}

	log.Printf("liveStreamId: %s", liveStreamID)
	log.Printf("WSS URL: %s", wssURL)

	// 连接 WebSocket
	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	headers.Set("Origin", "https://live.kuaishou.com")

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(wssURL, headers)
	if err != nil {
		log.Fatalf("WebSocket 连接失败: %v", err)
	}
	defer conn.Close()
	log.Println("WebSocket 连接成功")

	// 发送进房消息
	enterMsg, err := buildEnterRoomMsg(token, liveStreamID)
	if err != nil {
		log.Fatalf("构建进房消息失败: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, enterMsg); err != nil {
		log.Fatalf("发送进房消息失败: %v", err)
	}
	log.Println("已发送进房消息")

	// 启动心跳
	stopHB := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopHB:
				return
			case <-ticker.C:
				hbMsg, err := buildHeartbeatMsg()
				if err != nil {
					log.Printf("构建心跳消息失败: %v", err)
					continue
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, hbMsg); err != nil {
					log.Printf("发送心跳失败: %v", err)
					return
				}
			}
		}
	}()

	// 监听退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	// 接收消息
	msgCh := make(chan []byte, 100)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				if !strings.Contains(err.Error(), "close") {
					log.Printf("读取消息失败: %v", err)
				}
				close(msgCh)
				return
			}
			msgCh <- data
		}
	}()

	log.Println("开始监听快手直播间消息...")
	for {
		select {
		case <-sigCh:
			log.Println("收到退出信号，正在关闭...")
			close(stopHB)
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		case data, ok := <-msgCh:
			if !ok {
				log.Println("连接已断开")
				close(stopHB)
				return
			}
			handleSocketMessage(data)
		}
	}
}
