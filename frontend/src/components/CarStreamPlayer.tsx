import { Modal, Spin, Typography } from "antd";
import { useEffect, useRef, useState } from "react";
import {
  StartCarStream,
  StopCarStream,
} from "../../wailsjs/go/main/App";
import type { LogLevel } from "../types";

const { Text } = Typography;

interface CarStreamPlayerProps {
  ip: string;
  open: boolean;
  onClose: () => void;
  onLog?: (level: LogLevel, msg: string, detail?: string) => void;
}

// 等本地 ICE 收集完成 (无 STUN/TURN 时几乎瞬间完成)
function waitIceGathering(pc: RTCPeerConnection): Promise<void> {
  return new Promise((resolve) => {
    if (pc.iceGatheringState === "complete") return resolve();
    const handler = () => {
      if (pc.iceGatheringState === "complete") {
        pc.removeEventListener("icegatheringstatechange", handler);
        resolve();
      }
    };
    pc.addEventListener("icegatheringstatechange", handler);
  });
}

export function CarStreamPlayer({
  ip,
  open,
  onClose,
  onLog,
}: CarStreamPlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const [phase, setPhase] = useState<"idle" | "connecting" | "playing" | "error">(
    "idle",
  );
  const [errorText, setErrorText] = useState("");

  useEffect(() => {
    if (!open || !ip) return;
    let cancelled = false;

    const pc = new RTCPeerConnection({ iceServers: [] });
    pcRef.current = pc;
    setPhase("connecting");
    setErrorText("");

    // 我们只收流,不推流
    pc.addTransceiver("video", { direction: "recvonly" });
    pc.addTransceiver("audio", { direction: "recvonly" });

    pc.ontrack = (e) => {
      if (videoRef.current && e.streams[0]) {
        videoRef.current.srcObject = e.streams[0];
      }
    };
    pc.onconnectionstatechange = () => {
      if (pc.connectionState === "connected") setPhase("playing");
      if (
        pc.connectionState === "failed" ||
        pc.connectionState === "disconnected" ||
        pc.connectionState === "closed"
      ) {
        if (!cancelled) {
          setPhase("error");
          setErrorText(`PeerConnection ${pc.connectionState}`);
          onLog?.("warn", `视频流连接异常 (${pc.connectionState})`);
        }
      }
    };

    (async () => {
      try {
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        await waitIceGathering(pc);
        if (cancelled) return;

        const answer = await StartCarStream(ip, pc.localDescription!.sdp);
        if (cancelled) return;
        await pc.setRemoteDescription({ type: "answer", sdp: answer });
        onLog?.("info", `视频流已建立 (${ip})`);
      } catch (e: any) {
        if (cancelled) return;
        const msg = e?.message || String(e);
        setPhase("error");
        setErrorText(msg);
        onLog?.("error", `建立视频流失败 (${ip})`, msg);
      }
    })();

    return () => {
      cancelled = true;
      try {
        pc.getSenders().forEach((s) => s.track && s.track.stop());
      } catch {}
      pc.close();
      pcRef.current = null;
      StopCarStream().catch(() => {});
      if (videoRef.current) {
        videoRef.current.srcObject = null;
      }
    };
  }, [open, ip, onLog]);

  return (
    <Modal
      title={`整蛊设备视频 · ${ip}`}
      open={open}
      onCancel={onClose}
      footer={null}
      width={880}
      destroyOnClose
      maskClosable={false}
    >
      <div
        style={{
          position: "relative",
          background: "#000",
          borderRadius: 4,
          overflow: "hidden",
          minHeight: 320,
        }}
      >
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          controls
          style={{ width: "100%", maxHeight: 540, background: "#000" }}
        />
        {phase === "connecting" && (
          <div
            style={{
              position: "absolute",
              inset: 0,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              color: "#fff",
              background: "rgba(0,0,0,0.4)",
              gap: 8,
            }}
          >
            <Spin />
            <Text style={{ color: "#fff" }}>正在拉取整蛊设备视频流...</Text>
          </div>
        )}
        {phase === "error" && (
          <div
            style={{
              position: "absolute",
              inset: 0,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              color: "#fff",
              background: "rgba(0,0,0,0.6)",
              padding: 16,
              textAlign: "center",
              gap: 6,
            }}
          >
            <Text type="danger" style={{ color: "#ff7875", fontSize: 14 }}>
              获取视频连接失败,请确保整蛊设备在线或尝试重启设备
            </Text>
            {errorText && (
              <Text
                style={{
                  color: "#bfbfbf",
                  fontSize: 11,
                  wordBreak: "break-all",
                  whiteSpace: "pre-wrap",
                }}
              >
                技术细节:{errorText}
              </Text>
            )}
          </div>
        )}
      </div>
      <div style={{ marginTop: 8, fontSize: 12, color: "#999" }}>
        音频默认静音,可通过播放器控件取消静音。
      </div>
    </Modal>
  );
}
