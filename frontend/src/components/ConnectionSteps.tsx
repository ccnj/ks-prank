import { Steps } from "antd";

interface ConnectionStepsProps {
  status: string;
}

export function ConnectionSteps({ status }: ConnectionStepsProps) {
  let current = 0;
  let stepStatus: "wait" | "process" | "finish" | "error" = "wait";

  switch (status) {
    case "connecting":
      current = 0;
      stepStatus = "process";
      break;
    case "fetching_token":
      current = 1;
      stepStatus = "process";
      break;
    case "connected":
      current = 2;
      stepStatus = "finish";
      break;
    default:
      current = 0;
      stepStatus = "wait";
      break;
  }

  return (
    <Steps
      size="small"
      current={current}
      status={stepStatus}
      direction="vertical"
      items={[
        { title: "MQTT", description: "连接消息总线" },
        { title: "WSS Token", description: "获取直播间地址" },
        { title: "客户端就绪", description: "开始监听事件" },
      ]}
    />
  );
}
