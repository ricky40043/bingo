import { defineStore } from "pinia";
import { ref } from "vue";
export interface Settings {
  size: number;
  theme: string;
  target: number;
  winners: number;
  items: string[];
  joinMode: "auto" | "name";
  mode: "social" | "caller" | "turns";
  groups: number;
}
export interface Signature {
  name: string;
  ink?: string;
}
export interface Player {
  id: string;
  name: string;
  lines: number;
  cells: number[];
  count: number;
  rank: number;
  finishedAt?: number;
  online: boolean;
  group?: string;
  ready: boolean;
}
export interface TurnProposal {
  id: string;
  playerId: string;
  playerName: string;
  group: string;
  answer: string;
}
export interface MatchAppeal {
  id: string;
  playerId: string;
  playerName: string;
  group: string;
  index: number;
  answer: string;
  called: string;
}
export interface Room {
  id: string;
  settings: Settings;
  board: string[];
  status: "lobby" | "playing" | "finished";
  players: Player[];
  winners: Player[];
  startedAt: number;
  round: number;
  hostOnline: boolean;
  called: string[];
  turnIndex: number;
  pending?: TurnProposal;
  appeals: MatchAppeal[];
  approvedMatches: Record<string, string>;
}
interface Session {
  room: string;
  token: string;
  host: boolean;
}
export const useGame = defineStore("game", () => {
  const room = ref<Room | null>(null),
    me = ref<{
      id: string;
      marks: Record<number, Signature>;
      answers: Record<number, string>;
    } | null>(null),
    host = ref(false),
    connected = ref(false),
    ready = ref(false),
    error = ref(""),
    busy = ref(false),
    fatal = ref(false),
    winnerBoards = ref<Record<string, Record<number, Signature>>>({}),
    playerAnswers = ref<Record<string, Record<number, string>>>({});
  let socket: WebSocket | undefined,
    session: Session | undefined,
    retry: ReturnType<typeof setTimeout> | undefined,
    pendingTimer: ReturnType<typeof setTimeout> | undefined,
    leaveReturnUrl = "/";
  function settle() {
    busy.value = false;
    clearTimeout(pendingTimer);
  }
  function connect() {
    socket = new WebSocket(
      `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws`,
    );
    socket.onopen = () => {
      connected.value = true;
      if (session) {
        socket!.send(JSON.stringify({ type: "resume", ...session }));
      } else ready.value = true;
    };
    socket.onclose = () => {
      connected.value = false;
      ready.value = false;
      settle();
      retry = setTimeout(connect, 1500);
    };
    socket.onerror = () => {
      error.value = "連線中斷，正在重新連線…";
    };
    socket.onmessage = (event) => {
      const data = JSON.parse(event.data);
      if (data.type === "left") {
        if (session)
          sessionStorage.removeItem(
            `bingo:${session.room}:${session.host ? "host" : "player"}`,
          );
        session = undefined;
        settle();
        location.href = leaveReturnUrl;
        return;
      }
      if (data.type === "error") {
        error.value = data.message;
        settle();
        if (session && !ready.value) {
          fatal.value = true;
          session = undefined;
          ready.value = true;
        }
        return;
      }
      if (data.type === "session") {
        session = { room: data.room, token: data.token, host: data.host };
        sessionStorage.setItem(
          `bingo:${data.room}:${data.host ? "host" : "player"}`,
          JSON.stringify(session),
        );
        history.replaceState(
          {},
          "",
          `/?${data.host ? "host" : "room"}=${data.room}`,
        );
      }
      if (data.type === "state") {
        room.value = data.room;
        me.value = data.me;
        host.value = data.host;
        winnerBoards.value = data.winnerBoards || {};
        playerAnswers.value = data.playerAnswers || {};
        ready.value = true;
        error.value = "";
        settle();
      }
    };
  }
  function init() {
    const q = new URLSearchParams(location.search);
    const id = q.get("host") || q.get("room");
    if (id) {
      try {
        session =
          JSON.parse(
            sessionStorage.getItem(
              `bingo:${id}:${q.has("host") ? "host" : "player"}`,
            ) || "null",
          ) || undefined;
      } catch {
        session = undefined;
      }
    }
    connect();
  }
  function send(data: object) {
    if (!ready.value || socket?.readyState !== WebSocket.OPEN) {
      error.value = "請等待連線恢復後再試";
      return false;
    }
    if (busy.value) return false;
    busy.value = true;
    error.value = "";
    socket.send(JSON.stringify(data));
    pendingTimer = setTimeout(() => {
      settle();
      error.value = "尚未收到確認，正在重新連線取得最新進度";
      socket?.close();
    }, 10000);
    return true;
  }
  function leave() {
    clearTimeout(retry);
    clearTimeout(pendingTimer);
    if (session) {
      sessionStorage.removeItem(
        `bingo:${session.room}:${session.host ? "host" : "player"}`,
      );
      session = undefined;
    }
    if (socket) {
      socket.onclose = null;
      socket.close();
    }
    location.href = "/";
  }
  function leaveRoom(returnUrl: string) {
    leaveReturnUrl = returnUrl;
    send({ type: "leave" });
  }
  return {
    room,
    me,
    host,
    connected,
    ready,
    error,
    busy,
    fatal,
    winnerBoards,
    playerAnswers,
    init,
    send,
    leave,
    leaveRoom,
  };
});
