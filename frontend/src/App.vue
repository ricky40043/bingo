<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import QRCode from "qrcode";
import confetti from "canvas-confetti";
import { useGame, type Settings, type Player } from "./store";
import SignaturePad from "./components/SignaturePad.vue";
const game = useGame();
const initialQuery = new URLSearchParams(location.search);
const page = ref(initialQuery.has("room") ? "join" : "create");
const autoJoin = initialQuery.get("join") === "auto";
const linkedMode = initialQuery.get("mode");
const linkedGroups = Number(initialQuery.get("groups") || 0);
let autoJoinAttempted = false;
const roomCode = ref(initialQuery.get("room") || ""),
  playerName = ref(""),
  selectedGroup = ref("");
const roomInfo = ref<{
  mode: "social" | "caller";
  joinMode: "auto" | "name";
  groups: number;
  theme: string;
} | null>(null);
const settings = ref<Settings>({
  size: 4,
  theme: "運動",
  target: 2,
  winners: 3,
  items: [],
  joinMode: "auto",
  mode: "social",
  groups: 4,
});
const custom = ref(false),
  customItems = ref(""),
  customTheme = ref(""),
  themeBank = ref<Record<string, string[]>>({}),
  baseUrl = ref(location.origin);
const qr = ref(""),
  copied = ref(false),
  selected = ref<number | null>(null),
  popup = ref<Player | null>(null),
  now = ref(Date.now()),
  sound = ref(true),
  endConfirm = ref(false),
  inspectedWinner = ref<Player | null>(null),
  answerCell = ref<number | null>(null),
  answerValue = ref(""),
  callValue = ref("");
let timer: ReturnType<typeof setInterval>,
  popupTimer: ReturnType<typeof setTimeout>,
  audio: AudioContext | undefined;
let lastRound = "",
  seen = new Set<string>(),
  celebrations: Player[] = [];
const current = computed(() =>
  game.room?.players.find((p) => p.id === game.me?.id),
);
const joiningCaller = computed(
  () => linkedMode === "caller" || roomInfo.value?.mode === "caller",
);
const joiningAutomatically = computed(
  () => autoJoin || roomInfo.value?.joinMode === "auto",
);
const joiningGroups = computed(() =>
  Array.from(
    { length: linkedGroups || roomInfo.value?.groups || 0 },
    (_, index) => `${String.fromCharCode(65 + index)}組`,
  ),
);
const rankList = computed(() =>
  [...(game.room?.players || [])].sort(
    (a, b) => b.lines - a.lines || b.count - a.count,
  ),
);
const joinUrl = computed(() => {
  if (!game.room) return "";
  const query = new URLSearchParams({ room: game.room.id });
  if (game.room.settings.joinMode === "auto") query.set("join", "auto");
  if (game.room.settings.mode === "caller") {
    query.set("mode", "caller");
    query.set("groups", String(game.room.settings.groups));
  }
  return `${baseUrl.value.replace(/\/$/, "")}/?${query.toString()}`;
});
const elapsed = computed(() => {
  const ms = game.room?.startedAt ? now.value - game.room.startedAt : 0;
  return `${Math.floor(ms / 60000)
    .toString()
    .padStart(2, "0")}:${Math.floor((ms / 1000) % 60)
    .toString()
    .padStart(2, "0")}`;
});
const questionCount = computed(
  () =>
    new Set(
      customItems.value
        .split("\n")
        .map((x) => x.trim())
        .filter(Boolean),
    ).size,
);
const example = computed(() => {
  const pool = themeBank.value[settings.value.theme] || [
    "喜歡打籃球",
    "會游泳",
    "喜歡爬山",
    "喜歡跳舞",
    "會跳繩",
    "打過羽毛球",
    "喜歡散步",
    "踢過足球",
    "打過桌球",
    "喜歡騎腳踏車",
    "參加過路跑",
    "練過瑜珈",
    "玩過飛盤",
    "去過健身房",
    "有晨跑習慣",
    "參加過運動會",
  ];
  return (
    custom.value
      ? customItems.value
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean)
      : pool
  ).slice(0, settings.value.size ** 2);
});
const canSign = computed(
  () =>
    game.room?.status === "playing" &&
    game.ready &&
    !game.busy &&
    !current.value?.rank,
);
const callerMode = computed(() => game.room?.settings.mode === "caller");
const allCallerPlayersReady = computed(
  () =>
    !callerMode.value ||
    (!!game.room?.players.length && game.room.players.every((p) => p.ready)),
);
const normalizedCalled = computed(
  () => new Set((game.room?.called || []).map((value) => normalize(value))),
);
const hist = computed(() =>
  Array.from({ length: (game.room?.settings.size || 4) * 2 + 3 }, (_, i) => ({
    lines: i,
    count: game.room?.players.filter((p) => p.lines === i).length || 0,
  })).filter((x) => x.count || x.lines === 1 || x.lines === 2 || x.lines === 3),
);
const loopback = computed(() =>
  /^https?:\/\/(localhost|127\.0\.0\.1)(:|\/|$)/.test(baseUrl.value),
);
watch(
  () => settings.value.size,
  () => {
    settings.value.target = Math.min(
      settings.value.target,
      settings.value.size * 2 + 2,
    );
  },
);
watch(
  () => settings.value.mode,
  (mode) => {
    if (mode === "caller") {
      settings.value.size = 3;
      settings.value.theme = "大專學校";
      custom.value = false;
    } else if (
      ["大專學校", "名人", "大台北捷運站"].includes(settings.value.theme)
    ) {
      settings.value.theme = "運動";
    }
  },
);
watch(joinUrl, async (url) => {
  if (url)
    qr.value = await QRCode.toDataURL(url, {
      width: 280,
      margin: 2,
      color: { dark: "#243c31", light: "#ffffff" },
    });
});
function initAudio() {
  if (!audio) audio = new AudioContext();
  void audio.resume();
}
function bong() {
  if (!sound.value || !audio) return;
  const t = audio.currentTime;
  [392, 523.25, 783.99].forEach((freq, i) => {
    const o = audio!.createOscillator(),
      g = audio!.createGain();
    o.type = "sine";
    o.frequency.value = freq;
    o.connect(g);
    g.connect(audio!.destination);
    g.gain.setValueAtTime(0.16, t + i * 0.12);
    g.gain.exponentialRampToValueAtTime(0.001, t + i * 0.12 + 0.8);
    o.start(t + i * 0.12);
    o.stop(t + i * 0.12 + 0.85);
  });
}
function celebrateNext() {
  if (popup.value || !celebrations.length) return;
  popup.value = celebrations.shift()!;
  bong();
  if (!matchMedia("(prefers-reduced-motion: reduce)").matches)
    void confetti({
      particleCount: 100,
      spread: 95,
      origin: { y: 0.6 },
      colors: ["#dce887", "#324837", "#ff9776"],
    });
  popupTimer = setTimeout(() => {
    popup.value = null;
    celebrateNext();
  }, 3500);
}
watch(
  () => game.room,
  (room) => {
    if (!room) return;
    const round = `${room.id}:${room.round}`;
    if (round !== lastRound) {
      lastRound = round;
      seen = new Set(room.winners.map((p) => p.id));
      celebrations = [];
      clearTimeout(popupTimer);
      popup.value = null;
      selected.value = null;
      answerCell.value = null;
      inspectedWinner.value = null;
      return;
    }
    for (const p of room.winners) {
      if (!seen.has(p.id)) {
        seen.add(p.id);
        if (game.host || p.id === game.me?.id) celebrations.push(p);
      }
    }
    celebrateNext();
    if (room.status !== "playing") selected.value = null;
    if (selected.value !== null && game.me?.marks[selected.value])
      selected.value = null;
    if (
      answerCell.value !== null &&
      game.me?.answers[answerCell.value] === answerValue.value.trim()
    )
      answerCell.value = null;
  },
);
function create() {
  game.send({
    type: "create",
    settings: {
      ...settings.value,
      theme: custom.value ? customTheme.value : settings.value.theme,
      items: custom.value ? customItems.value.split("\n") : [],
    },
  });
}
function join() {
  game.send({
    type: "join",
    room: roomCode.value.toUpperCase(),
    name: playerName.value,
    group: selectedGroup.value,
  });
}
watch(
  [() => game.ready, joiningAutomatically, joiningCaller, roomCode],
  ([ready]) => {
    if (
      !ready ||
      !joiningAutomatically.value ||
      joiningCaller.value ||
      autoJoinAttempted ||
      game.room
    )
      return;
    autoJoinAttempted = true;
    join();
  },
);
watch(
  roomCode,
  async (code) => {
    const normalized = code.trim().toUpperCase();
    if (normalized.length !== 6 || linkedMode) return;
    try {
      const response = await fetch(
        `/api/rooms/${encodeURIComponent(normalized)}`,
      );
      roomInfo.value = response.ok ? await response.json() : null;
    } catch {
      roomInfo.value = null;
    }
  },
  { immediate: true },
);
function start() {
  initAudio();
  game.send({ type: "start" });
}
async function copy() {
  try {
    await navigator.clipboard.writeText(joinUrl.value);
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    game.error = "無法自動複製，請選取下方網址複製";
  }
}
function sign(value: { name: string; ink: string }) {
  game.send({ type: "sign", index: selected.value, signature: value });
}
function normalize(value: string) {
  return value
    .trim()
    .toLocaleLowerCase()
    .replace(/\s+/g, "")
    .replace(/臺/g, "台");
}
function openAnswer(index: number) {
  answerCell.value = index;
  answerValue.value = game.me?.answers[index] || "";
}
function saveAnswer() {
  if (answerCell.value === null || !answerValue.value.trim()) return;
  game.send({
    type: "answer",
    index: answerCell.value,
    value: answerValue.value,
  });
}
function publishCall() {
  if (!callValue.value.trim()) return;
  if (game.send({ type: "call", value: callValue.value })) callValue.value = "";
}
function isClaimable(index: number) {
  const answer = game.me?.answers[index];
  return !!answer && normalizedCalled.value.has(normalize(answer));
}
function claim(index: number) {
  if (isClaimable(index) && !game.me?.marks[index]) {
    game.send({ type: "claim", index });
  }
}
function chooseGroup(group: string) {
  selectedGroup.value = group;
  if (joiningAutomatically.value) join();
}
function rechooseGroup() {
  if (!game.room) return;
  game.leaveRoom(joinUrl.value);
}
function duration(p: Player) {
  return p.finishedAt && game.room
    ? `${((p.finishedAt - game.room.startedAt) / 1000).toFixed(1)} 秒`
    : "—";
}
function inspectWinner(player: Player) {
  if (!callerMode.value && player.rank > 0) inspectedWinner.value = player;
}
onMounted(async () => {
  game.init();
  timer = setInterval(() => (now.value = Date.now()), 1000);
  try {
    const [themes, config] = await Promise.all([
      fetch("/api/themes").then((r) => r.json()),
      fetch("/api/config").then((r) => r.json()),
    ]);
    themeBank.value = themes;
    if (config.frontendUrl) baseUrl.value = config.frontendUrl;
  } catch {
    game.error = "題庫讀取失敗，請確認後端服務後重新整理";
  }
});
onUnmounted(() => {
  clearInterval(timer);
  clearTimeout(popupTimer);
});
</script>
<template>
  <div class="app-shell" :class="{ 'player-shell': game.room && !game.host }">
    <header class="topbar">
      <div class="logo" aria-label="Bingo"
        ><span class="logo-mark">b.</span>bingo<span class="logo-dot"
          >!</span
        ></div
      ><span class="top-tag">讓每一次相遇，連成一線。</span
      ><span class="connection" :class="{ offline: !game.ready }"
        ><i />{{ game.ready ? "即時連線中" : "正在連線…" }}</span
      >
    </header>
    <div v-if="game.error" class="notice error" role="alert">
      {{ game.error
      }}<button v-if="game.fatal" @click="game.leave">回到首頁</button
      ><button v-else aria-label="關閉提示" @click="game.error = ''">×</button>
    </div>
    <template v-if="!game.room">
      <main v-if="page === 'create'" class="setup-layout">
        <section class="intro">
          <p class="eyebrow">
            <span class="tiny-star">✳</span> PEOPLE, MEET PEOPLE.
          </p>
          <h1>一個簽名，<br />多一點<span class="highlight">連結。</span></h1>
          <p class="intro-description">
            走近一個人，找到共同點。<br />把朋友的名字，連成你的 Bingo！
          </p>
          <div class="demo-wrap">
            <div class="demo-label">
              <span>✦ YOUR NEXT CONNECTION</span
              ><span>{{ settings.size }} × {{ settings.size }}</span>
            </div>
            <div
              class="bingo-grid demo-grid"
              :style="{ '--size': settings.size }"
            >
              <div
                v-for="(item, i) in example"
                :key="i"
                class="cell"
                :class="{
                  signed: i === 0 || i === 5 || i === 10,
                  lined: i === 0,
                }"
              >
                <span class="cell-number">{{
                  String(i + 1).padStart(2, "0")
                }}</span
                ><strong>{{ item }}</strong
                ><span v-if="i === 0 || i === 5 || i === 10" class="demo-sign"
                  >{{ i === 0 ? "Jamie" : i === 5 ? "小安" : "Chris" }} ✓</span
                ><span v-else class="cell-plus">＋</span>
              </div>
            </div>
            <span class="sticker">Say hello<br />then BINGO!</span>
          </div>
          <div class="how-it-works">
            <span><b>01</b> 掃碼加入</span><span><b>02</b> 找朋友簽名</span
            ><span><b>03</b> 連線得獎</span>
          </div>
        </section>
        <section class="setup-card">
          <div class="card-heading">
            <span class="mini-label">HOST A GAME</span>
            <h2>讓大家，玩在一起。</h2>
            <p>設定這一場相遇的方式。</p>
          </div>
          <form @submit.prevent="create">
            <fieldset>
              <legend>01 <span>選擇玩法</span></legend>
              <div class="choice-row mode-choices">
                <button
                  type="button"
                  class="size-choice"
                  :class="{ active: settings.mode === 'social' }"
                  @click="settings.mode = 'social'"
                >
                  <span class="grid-icon">✍</span><b>互相簽名</b
                  ><small>找朋友完成任務</small>
                </button>
                <button
                  type="button"
                  class="size-choice"
                  :class="{ active: settings.mode === 'caller' }"
                  @click="settings.mode = 'caller'"
                >
                  <span class="grid-icon">◎</span><b>分組猜答案</b
                  ><small>填九宮格、聽題圈選</small>
                </button>
              </div>
            </fieldset>
            <fieldset v-if="settings.mode === 'social'">
              <legend>02 <span>選擇格子大小</span></legend>
              <div class="choice-row">
                <button
                  type="button"
                  class="size-choice"
                  :class="{ active: settings.size === 3 }"
                  @click="settings.size = 3"
                >
                  <span class="grid-icon">▦</span><b>九宮格</b
                  ><small>3 × 3 · 輕鬆暖場</small></button
                ><button
                  type="button"
                  class="size-choice"
                  :class="{ active: settings.size === 4 }"
                  @click="settings.size = 4"
                >
                  <span class="grid-icon">▦</span><b>十六宮格</b
                  ><small>4 × 4 · 深度交流</small>
                </button>
              </div>
            </fieldset>
            <fieldset>
              <legend>
                03
                <span>{{
                  settings.mode === "caller" ? "選擇題目類別" : "今天聊什麼？"
                }}</span>
              </legend>
              <template v-if="settings.mode === 'caller'">
                <label class="sr-only" for="caller-theme">題目類別</label>
                <select id="caller-theme" v-model="settings.theme">
                  <option value="大專學校">🎓 大專學校</option>
                  <option value="名人">🌟 名人</option>
                  <option value="大台北捷運站">🚇 大台北捷運站</option>
                </select>
                <label
                  >分成幾組？
                  <div class="unit-input">
                    <input
                      v-model.number="settings.groups"
                      type="number"
                      min="2"
                      max="12"
                      required
                    />
                    <span>組</span>
                  </div>
                </label>
                <p class="field-help">
                  玩家會依加入順序平均分到
                  A、B、C…組，並在自己的手機填寫九個答案。
                </p>
              </template>
              <template v-else>
                <label class="sr-only" for="theme">題目主題</label
                ><select
                  id="theme"
                  :value="custom ? 'custom' : settings.theme"
                  @change="
                    custom =
                      ($event.target as HTMLSelectElement).value === 'custom';
                    if (!custom)
                      settings.theme = (
                        $event.target as HTMLSelectElement
                      ).value;
                  "
                >
                  <option value="運動">🏃 運動 · 動起來，認識你</option>
                  <option value="生活與興趣">☕ 生活與興趣 · 分享日常</option>
                  <option value="認識彼此">👋 認識彼此 · 從你好開始</option>
                  <option value="custom">✎ 自訂主題與固定題庫</option>
                </select>
                <template v-if="custom"
                  ><label
                    >主題名稱<input
                      v-model="customTheme"
                      placeholder="例如：旅行的回憶"
                      required
                      maxlength="80" /></label
                  ><label
                    >題目清單 · 每行一題<textarea
                      v-model="customItems"
                      rows="5"
                      placeholder="去過日本&#10;喜歡自助旅行&#10;曾經一個人旅行"
                      required
                    /></label
                  ><small
                    :class="{ invalid: questionCount < settings.size ** 2 }"
                    >{{ questionCount }} 個不重複題目 / 至少
                    {{ settings.size ** 2 }} 題</small
                  ></template
                >
                <p class="field-help">
                  每場隨機抽出
                  {{ settings.size ** 2 }} 題，所有人的題目和位置完全相同。
                </p>
              </template>
            </fieldset>
            <fieldset>
              <legend>04 <span>大家怎麼加入？</span></legend>
              <div class="choice-row">
                <button
                  type="button"
                  class="size-choice"
                  :class="{ active: settings.joinMode === 'auto' }"
                  @click="settings.joinMode = 'auto'"
                >
                  <span class="grid-icon">⚡</span><b>點連結直接加入</b
                  ><small>自動產生動物暱稱</small>
                </button>
                <button
                  type="button"
                  class="size-choice"
                  :class="{ active: settings.joinMode === 'name' }"
                  @click="settings.joinMode = 'name'"
                >
                  <span class="grid-icon">✎</span><b>輸入名字加入</b
                  ><small>玩家自己填寫名字</small>
                </button>
              </div>
              <p class="field-help" v-if="settings.joinMode === 'auto'">
                玩家點開邀請連結後直接進房，系統會取名，例如「生氣的獅子」或「憂鬱的羚羊」。
              </p>
              <p class="field-help" v-else>
                玩家點開邀請連結後，輸入名字才會加入房間。
              </p>
            </fieldset>
            <fieldset>
              <legend>05 <span>怎麼算 Bingo？</span></legend>
              <div class="rule-inputs">
                <label
                  >達標線數
                  <div class="unit-input">
                    <input
                      v-model.number="settings.target"
                      aria-label="達標線數"
                      type="number"
                      min="1"
                      :max="settings.size * 2 + 2"
                      required
                    /><span>條線</span>
                  </div></label
                ><label
                  >得獎名額
                  <div class="unit-input">
                    <input
                      v-model.number="settings.winners"
                      aria-label="得獎名額"
                      type="number"
                      min="1"
                      max="100"
                      required
                    /><span>位</span>
                  </div></label
                >
              </div>
              <p class="field-help">
                橫、直、對角線皆算。前 {{ settings.winners }} 位達成
                {{ settings.target }} 條線即結束。
              </p>
            </fieldset>
            <button
              class="primary full"
              :disabled="
                !game.ready ||
                game.busy ||
                (custom && questionCount < settings.size ** 2)
              "
            >
              {{ game.busy ? "建立中…" : "建立遊戲房間" }} <span>↗</span>
            </button>
            <p class="under-button">免下載 App，手機掃碼就能玩。</p>
          </form>
          <div class="join-switch">
            來參加活動？<button class="text-button" @click="page = 'join'">
              輸入房號加入 →
            </button>
          </div>
        </section>
      </main>
      <main v-else class="join-page">
        <div class="join-art">✳</div>
        <p class="eyebrow">A NEW CONNECTION STARTS HERE</p>
        <template v-if="joiningCaller">
          <h1>你是哪一組？</h1>
          <p class="muted">選擇組別後，就會加入這場遊戲。每組人數不限。</p>
          <div v-if="!joiningAutomatically" class="join-card group-name-card">
            <label
              >房間代碼<input
                v-model="roomCode"
                placeholder="6 碼房號"
                maxlength="6"
                minlength="6"
                required
                autocapitalize="characters"
                class="room-input"
            /></label>
            <label
              >你的名字<input
                v-model="playerName"
                placeholder="想讓大家怎麼稱呼你？"
                required
                maxlength="80"
                autocomplete="nickname"
            /></label>
          </div>
          <div class="group-picker" aria-label="選擇組別">
            <button
              v-for="group in joiningGroups"
              :key="group"
              type="button"
              :class="{ active: selectedGroup === group }"
              :disabled="game.busy"
              @click="
                joiningAutomatically
                  ? chooseGroup(group)
                  : (selectedGroup = group)
              "
            >
              <strong>{{ group.replace("組", "") }}</strong
              ><span>{{ group }}</span>
            </button>
          </div>
          <button
            v-if="!joiningAutomatically"
            class="primary full group-join-button"
            :disabled="
              !game.ready ||
              game.busy ||
              !selectedGroup ||
              !playerName.trim() ||
              roomCode.length !== 6
            "
            @click="join"
          >
            {{
              game.busy ? "加入中…" : `加入 ${selectedGroup || "選擇的組別"} →`
            }}
          </button>
          <p v-if="!joiningGroups.length" class="notice">
            請先輸入有效房號，讀取可選組別。
          </p>
        </template>
        <template v-else-if="joiningAutomatically">
          <h1>正在幫你<br />加入遊戲。</h1>
          <p class="muted">系統正在準備你的動物暱稱…</p>
          <div class="waiting-dots"><i /><i /><i /></div>
        </template>
        <template v-else>
          <h1>嗨，很高興<br />在這裡遇見你。</h1>
          <p class="muted">填上名字，準備好和新朋友說聲你好。</p>
          <form class="join-card" @submit.prevent="join">
            <label
              >房間代碼<input
                v-model="roomCode"
                placeholder="6 碼房號"
                maxlength="6"
                minlength="6"
                required
                autocapitalize="characters"
                class="room-input" /></label
            ><label
              >你的名字<input
                v-model="playerName"
                placeholder="想讓大家怎麼稱呼你？"
                required
                maxlength="80"
                autocomplete="nickname" /></label
            ><small class="muted">名字可以重複，每個人都有自己的進度。</small
            ><button class="primary full" :disabled="!game.ready || game.busy">
              {{ game.busy ? "加入中…" : "加入這場相遇 →" }}
            </button>
          </form>
        </template>
        <button class="text-button" @click="page = 'create'">
          我是主持人，建立房間
        </button>
      </main>
    </template>
    <main v-else-if="game.host" class="host-page">
      <div class="room-heading">
        <div>
          <p class="eyebrow">
            {{
              game.room.status === "lobby"
                ? "THE ROOM IS OPEN"
                : game.room.status === "playing"
                  ? "LET’S MAKE CONNECTIONS"
                  : "THE CONNECTIONS WE MADE"
            }}
            · 第 {{ game.room.round }} 場
          </p>
          <h1>
            {{
              game.room.status === "lobby"
                ? "人到齊，就開始。"
                : game.room.status === "playing"
                  ? callerMode
                    ? "聽清楚，準備圈答案。"
                    : "好玩的相遇，正在發生。"
                  : "Bingo！你們做到了。"
            }}
          </h1>
          <p class="muted">
            {{ game.room.settings.theme }} · {{ game.room.settings.size }} ×
            {{ game.room.settings.size }} ·
            {{ game.room.settings.target }} 條線達標 · 前
            {{ game.room.settings.winners }} 名得獎
            <template v-if="callerMode">
              · {{ game.room.settings.groups }} 組</template
            >
          </p>
        </div>
        <div class="room-controls">
          <button
            v-if="game.room.status === 'lobby'"
            class="secondary"
            @click="game.leave()"
          >
            ← 回到主選單
          </button>
          <button
            class="secondary"
            @click="
              sound = !sound;
              initAudio();
            "
          >
            {{ sound ? "♫ 音效開啟" : "♫ 音效關閉" }}</button
          ><button
            v-if="game.room.status === 'playing'"
            class="secondary"
            @click="endConfirm = true"
          >
            提前結束</button
          ><button
            v-if="game.room.status === 'finished'"
            class="primary"
            :disabled="!game.ready || game.busy"
            @click="game.send({ type: 'reset' })"
          >
            再玩一場 ↻
          </button
          ><button
            v-if="game.room.status === 'finished'"
            class="secondary"
            @click="game.leave()"
          >
            回到主選單
          </button>
        </div>
      </div>
      <div v-if="game.room.status === 'finished'" class="results-banner">
        <p class="eyebrow">THE BINGO CLUB</p>
        <h2>把掌聲，送給今天的連線高手。</h2>
        <div class="podium">
          <button
            v-for="p in game.room.winners"
            :key="p.id"
            type="button"
            class="winner-card"
            :disabled="callerMode"
            @click="inspectWinner(p)"
          >
            <span class="medal"
              >{{ p.rank === 1 ? "✦" : p.rank === 2 ? "✧" : "✳"
              }}<small>{{ p.rank }}</small></span
            >
            <h3>{{ p.name }}</h3>
            <p>{{ p.lines }} 條線 · {{ duration(p) }}</p>
            <small v-if="!callerMode" class="review-hint">點擊查看簽名板</small>
          </button>
          <p v-if="!game.room.winners.length">本場提前結束，尚無玩家達標。</p>
        </div>
      </div>
      <div class="host-columns">
        <section class="board-panel">
          <div class="panel-title">
            <h2>
              {{
                game.room.status === "lobby"
                  ? callerMode
                    ? "玩家正在準備答案"
                    : "這一場的共同話題"
                  : callerMode
                    ? "主持人公布答案"
                    : "全場共用主板"
              }}
            </h2>
            <span class="chip">{{ game.room.settings.theme }}</span>
          </div>
          <p v-if="!callerMode" class="muted small">
            每支手機都是這份排列，找到符合描述的朋友，請他簽名。
          </p>
          <p v-else class="muted small">
            每位玩家有自己的九宮格；公布答案後，符合的玩家就能在手機上圈選。
          </p>
          <form
            v-if="callerMode && game.room.status === 'playing'"
            class="call-console"
            @submit.prevent="publishCall"
          >
            <label for="call-answer">這一題的答案</label>
            <div>
              <input
                id="call-answer"
                v-model="callValue"
                maxlength="40"
                :placeholder="`例如：${game.room.settings.theme === '大專學校' ? '臺灣大學' : game.room.settings.theme === '名人' ? '周杰倫' : '台北車站'}`"
                required
              />
              <button
                class="primary"
                :disabled="game.busy || !callValue.trim()"
              >
                公布答案 →
              </button>
            </div>
          </form>
          <div v-if="callerMode" class="called-board">
            <div v-if="!game.room.called.length" class="empty-state compact">
              ◎
              <p>
                {{
                  game.room.status === "lobby"
                    ? "等待玩家填滿九宮格"
                    : "還沒有公布答案"
                }}
              </p>
            </div>
            <ol v-else>
              <li
                v-for="(answer, index) in [...game.room.called].reverse()"
                :key="answer"
              >
                <span>{{ game.room.called.length - index }}</span
                ><strong>{{ answer }}</strong>
              </li>
            </ol>
          </div>
          <div
            v-else
            class="bingo-grid host-grid"
            :style="{ '--size': game.room.settings.size }"
          >
            <div v-for="(q, i) in game.room.board" :key="i" class="cell">
              <span class="cell-number">{{
                String(i + 1).padStart(2, "0")
              }}</span
              ><strong>{{ q }}</strong
              ><span class="cell-plus">✧</span>
            </div>
          </div>
          <div v-if="!callerMode" class="board-foot">
            <span>↔ 橫線</span><span>↕ 直線</span><span>⤢ 對角線</span
            ><span>都算一條連線</span>
          </div>
        </section>
        <aside class="live-sidebar">
          <section
            v-if="
              game.room.status === 'lobby' ||
              (!callerMode && game.room.status === 'playing')
            "
            class="qr-card"
          >
            <p class="eyebrow">SCAN, SAY HI & PLAY</p>
            <h2>掃一下，一起玩。</h2>
            <img
              v-if="qr"
              :src="qr"
              alt="掃描 QR Code 加入 Bingo 房間"
              width="200"
              height="200"
            />
            <p class="room-code">{{ game.room.id }}</p>
            <button class="secondary full" @click="copy">
              {{ copied ? "已複製 ✓" : "複製邀請連結 ↗" }}</button
            ><input
              class="share-url"
              aria-label="玩家加入網址"
              :value="joinUrl"
              readonly
              @focus="($event.target as HTMLInputElement).select()"
            />
            <p v-if="loopback" class="lan-tip">
              手機掃碼前，請用電腦的區網 IP 開啟本頁；手機與電腦需連同一個
              Wi-Fi。
            </p>
          </section>
          <section class="stats-card">
            <div class="stat-main">
              <div>
                <strong>{{ game.room.players.length }}</strong
                ><span
                  >位玩家 ·
                  {{ game.room.players.filter((p) => p.online).length }}
                  位在線</span
                >
              </div>
              <div v-if="game.room.status === 'playing'">
                <strong class="timer">{{ elapsed }}</strong
                ><span>遊戲進行中</span>
              </div>
              <div v-else>
                <strong
                  >{{ game.room.winners.length
                  }}<small>/{{ game.room.settings.winners }}</small></strong
                ><span>已達標</span>
              </div>
            </div>
            <div v-if="game.room.status !== 'lobby'" class="histogram">
              <div v-for="entry in hist" :key="entry.lines">
                <span>{{ entry.lines }} 條線</span>
                <div class="bar">
                  <i
                    :style="{
                      width: `${(entry.count / Math.max(1, game.room.players.length)) * 100}%`,
                    }"
                  />
                </div>
                <b>{{ entry.count }} 人</b>
              </div>
              <small>依目前完成線數統計，每人只列入一組。</small>
            </div>
            <button
              v-if="game.room.status === 'lobby'"
              class="primary full"
              :disabled="
                !game.ready ||
                game.busy ||
                game.room.players.length < game.room.settings.winners ||
                !allCallerPlayersReady
              "
              @click="start"
            >
              開始遊戲 <span>→</span>
            </button>
            <p v-if="game.room.status === 'lobby'" class="field-help">
              <template v-if="callerMode && !allCallerPlayersReady"
                >請等所有玩家填滿九宮格。</template
              >
              <template v-else
                >至少
                {{ game.room.settings.winners }} 位玩家加入即可開始。</template
              >
            </p>
          </section>
        </aside>
      </div>
      <section class="players-panel">
        <div class="panel-title">
          <h2>
            {{
              game.room.status === "lobby" ? "已經到場的朋友" : "即時連線進度"
            }}
          </h2>
          <span class="muted">{{ game.room.players.length }} 位玩家</span>
        </div>
        <div v-if="!game.room.players.length" class="empty-state">
          ✳
          <p>第一位朋友，會是誰呢？</p>
          <small>邀請大家掃描 QR Code 加入。</small>
        </div>
        <div v-else class="player-list">
          <article v-for="(p, i) in rankList" :key="p.id" class="player-row">
            <span class="avatar" :class="{ winner: p.rank }">{{
              p.rank ? "★" : p.name.slice(0, 1)
            }}</span>
            <div class="player-name">
              <strong>{{ p.name }}</strong
              ><small
                >{{ p.online ? "● 在線" : "○ 暫時離線" }} ·
                {{ p.id.slice(0, 4) }}</small
              >
            </div>
            <span v-if="p.group" class="group-badge">{{ p.group }}</span>
            <span
              v-if="callerMode && game.room.status === 'lobby'"
              :class="p.ready ? 'ready-badge' : 'muted'"
            >
              {{ p.ready ? "已填滿" : "填寫中" }}
            </span>
            <span v-if="p.rank" class="winner-badge">第 {{ p.rank }} 名</span
            ><span v-else class="muted">#{{ i + 1 }}</span
            ><span class="line-count">{{ p.lines }} <small>條線</small></span
            ><span class="muted"
              >{{ p.count }} / {{ game.room.board.length }} 格</span
            >
            <button
              v-if="p.rank && !callerMode"
              type="button"
              class="text-button"
              @click="inspectWinner(p)"
            >
              查看簽名板
            </button>
          </article>
        </div>
      </section>
    </main>
    <main v-else class="player-page">
      <div class="player-welcome">
        <span class="chip"
          >{{ game.room.settings.theme }} · {{ game.room.id
          }}<template v-if="current?.group">
            · {{ current.group }}</template
          ></span
        >
        <h1>嗨，{{ current?.name }} <span>✳</span></h1>
        <p v-if="game.room.status === 'lobby'">
          {{
            callerMode
              ? `你是 ${current?.group}，請先填滿自己的九宮格。`
              : "你已加入！等主持人開始，就去認識新朋友。"
          }}
        </p>
        <p v-else-if="game.room.status === 'finished'">
          這場相遇圓滿結束，謝謝你一起玩。
        </p>
        <p v-else-if="current?.rank">
          你是第 {{ current.rank }} 位 Bingo！等大家一起完成吧。
        </p>
        <p v-else>
          {{
            callerMode
              ? "主持人公布答案後，符合的格子就可以圈起來。"
              : "點一格，把手機交給符合描述的朋友簽名。"
          }}
        </p>
      </div>
      <div v-if="game.room.status === 'finished'" class="finished-actions">
        <button class="primary full" @click="game.leave()">
          回到主選單
        </button>
        <p class="player-hint">主持人開啟下一場時，此頁會自動更新。</p>
      </div>
      <div v-if="!game.room.hostOnline" class="notice">
        主持人暫時離線，房間與進度都會保留。
      </div>
      <button
        v-if="callerMode && game.room.status === 'lobby'"
        class="secondary rechoose-button"
        :disabled="game.busy"
        @click="rechooseGroup"
      >
        選錯組？退出並重新選組
      </button>
      <section
        v-if="callerMode && game.room.status === 'lobby'"
        class="answer-setup"
      >
        <div class="progress-card">
          <div>
            <strong
              >{{ Object.keys(game.me?.answers || {}).length
              }}<small> / 9 格</small></strong
            ><span>{{ current?.ready ? "準備完成 ✓" : "點格子填答案" }}</span>
          </div>
          <div class="progress-track">
            <i
              :style="{
                width: `${(Object.keys(game.me?.answers || {}).length / 9) * 100}%`,
              }"
            />
          </div>
        </div>
        <div class="bingo-grid player-grid answer-grid" style="--size: 3">
          <button
            v-for="i in 9"
            :key="i"
            class="cell"
            :class="{ filled: game.me?.answers[i - 1] }"
            :disabled="game.busy"
            @click="openAnswer(i - 1)"
          >
            <span class="cell-number">{{ String(i).padStart(2, "0") }}</span>
            <strong>{{ game.me?.answers[i - 1] || "填寫答案" }}</strong>
            <span class="cell-plus">{{
              game.me?.answers[i - 1] ? "✎" : "＋"
            }}</span>
          </button>
        </div>
        <p class="player-hint">同一個答案不能重複填寫 · 開始前都可以修改</p>
      </section>
      <div v-else-if="game.room.status === 'lobby'" class="waiting-card">
        <span class="waiting-symbol">✳</span>
        <h2>好玩的事，值得等一下。</h2>
        <p>{{ game.room.players.length }} 位朋友已到場</p>
        <div class="waiting-dots"><i /><i /><i /></div>
        <small
          >目標 {{ game.room.settings.target }} 條線 · 前
          {{ game.room.settings.winners }}
          位得獎<br />主持人按下開始後，格子會自動開放。</small
        >
      </div>
      <template v-else
        ><div class="progress-card">
          <div>
            <strong
              >{{ current?.lines || 0
              }}<small> / {{ game.room.settings.target }} 條線</small></strong
            ><span
              >{{ current?.count || 0 }} 格{{
                callerMode ? "已圈選" : "已簽名"
              }}</span
            >
          </div>
          <div class="progress-track">
            <i
              :style="{
                width: `${Math.min(100, ((current?.lines || 0) / game.room.settings.target) * 100)}%`,
              }"
            />
          </div>
        </div>
        <div v-if="callerMode && game.room.called.length" class="latest-call">
          <span>主持人剛公布</span
          ><strong>{{ game.room.called[game.room.called.length - 1] }}</strong>
        </div>
        <div
          class="bingo-grid player-grid"
          :style="{ '--size': callerMode ? 3 : game.room.settings.size }"
        >
          <button
            v-for="(q, i) in callerMode ? Array(9).fill('') : game.room.board"
            :key="i"
            class="cell"
            :class="{
              signed: game.me?.marks[i],
              lined: current?.cells.includes(i),
              claimable: callerMode && isClaimable(i) && !game.me?.marks[i],
            }"
            :disabled="
              callerMode
                ? !canSign || !isClaimable(i) || !!game.me?.marks[i]
                : !canSign || !!game.me?.marks[i]
            "
            :aria-label="
              callerMode
                ? `${game.me?.answers[i]}${game.me?.marks[i] ? '，已圈選' : isClaimable(i) ? '，可以圈選' : '，尚未公布'}`
                : `${q}${game.me?.marks[i] ? '，已簽名' : '，點擊簽名'}`
            "
            @click="callerMode ? claim(i) : (selected = i)"
          >
            <span class="cell-number">{{ String(i + 1).padStart(2, "0") }}</span
            ><strong>{{ callerMode ? game.me?.answers[i] : q }}</strong
            ><span v-if="callerMode && game.me?.marks[i]" class="signed-name"
              >◯ 已圈選</span
            ><img
              v-else-if="game.me?.marks[i]?.ink"
              class="signature-image"
              :src="game.me.marks[i].ink"
              alt="手寫簽名"
            />
            <span v-else class="cell-plus">＋</span>
          </button>
        </div>
        <p class="player-hint">
          橫、直、對角線都算 ·
          {{
            callerMode ? "亮起的答案可以點擊圈選" : "已簽名的格子會自動計算連線"
          }}
        </p>
        <section v-if="game.room.winners.length" class="mobile-winners">
          <p class="eyebrow">BINGO MOMENTS</p>
          <h2>
            {{
              game.room.status === "finished"
                ? "本場連線高手"
                : "有人 Bingo 了！"
            }}
          </h2>
          <article v-for="p in game.room.winners" :key="p.id">
            <b>{{ p.rank === 1 ? "★" : "✧" }} {{ p.rank }}</b
            ><strong>{{ p.name }}</strong
            ><span>{{ duration(p) }}</span>
          </article>
        </section>
      </template>
    </main>
    <footer>
      <span>BINGO! · BETTER TOGETHER</span><span>從一句你好，開始連結。</span>
    </footer>
    <SignaturePad
      v-if="selected !== null && game.room && !callerMode"
      :question="game.room.board[selected]"
      :busy="game.busy"
      @close="selected = null"
      @submit="sign"
    />
    <div
      v-if="inspectedWinner && game.room && game.host"
      class="overlay"
      @click.self="inspectedWinner = null"
    >
      <section
        class="signature-dialog board-review-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="review-title"
      >
        <button
          class="close-button"
          aria-label="關閉簽名板"
          @click="inspectedWinner = null"
        >
          ×
        </button>
        <p class="eyebrow">
          第 {{ inspectedWinner.rank }} 名 · SIGNATURE BOARD
        </p>
        <h2 id="review-title">{{ inspectedWinner.name }} 的簽名板</h2>
        <p class="muted">每一格都是朋友直接留下的原始手寫簽名。</p>
        <div
          class="bingo-grid review-grid"
          :style="{ '--size': game.room.settings.size }"
        >
          <div
            v-for="(question, index) in game.room.board"
            :key="index"
            class="cell"
            :class="{ signed: game.winnerBoards[inspectedWinner.id]?.[index] }"
          >
            <span class="cell-number">{{
              String(index + 1).padStart(2, "0")
            }}</span>
            <strong>{{ question }}</strong>
            <img
              v-if="game.winnerBoards[inspectedWinner.id]?.[index]?.ink"
              class="signature-image review-signature"
              :src="game.winnerBoards[inspectedWinner.id][index].ink"
              alt="手寫簽名"
            />
            <span v-else class="unsigned-cell">尚未簽名</span>
          </div>
        </div>
      </section>
    </div>
    <div
      v-if="answerCell !== null && callerMode"
      class="overlay"
      @click.self="answerCell = null"
    >
      <section
        class="signature-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="answer-title"
      >
        <button
          class="close-button"
          aria-label="關閉"
          :disabled="game.busy"
          @click="answerCell = null"
        >
          ×
        </button>
        <p class="eyebrow">
          {{ current?.group }} · {{ game.room?.settings.theme }}
        </p>
        <h2 id="answer-title">第 {{ (answerCell ?? 0) + 1 }} 格寫什麼？</h2>
        <form @submit.prevent="saveAnswer">
          <label
            >你的答案<input
              v-model="answerValue"
              autofocus
              maxlength="40"
              required
              :disabled="game.busy"
              placeholder="輸入一個答案"
          /></label>
          <button
            class="primary full"
            :disabled="game.busy || !answerValue.trim()"
          >
            {{ game.busy ? "儲存中…" : "放進九宮格 ✓" }}
          </button>
        </form>
      </section>
    </div>
    <div
      v-if="popup"
      class="overlay celebration"
      role="status"
      aria-live="polite"
    >
      <div class="winner-popup">
        <p class="eyebrow">WE HAVE A WINNER</p>
        <div class="burst">✳</div>
        <h2>BINGO!</h2>
        <h3>{{ popup.name }}</h3>
        <p>第 {{ popup.rank }} 位達標 · {{ popup.lines }} 條連線</p>
        <span>{{ duration(popup) }}</span>
      </div>
    </div>
    <div v-if="endConfirm" class="overlay" @click.self="endConfirm = false">
      <section
        class="signature-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="end-title"
      >
        <h2 id="end-title">要提前結束這一場嗎？</h2>
        <p>將保留目前得獎者，所有玩家停止簽名。</p>
        <div class="choice-row">
          <button class="secondary" @click="endConfirm = false">繼續遊戲</button
          ><button
            class="primary"
            :disabled="game.busy || !game.ready"
            @click="
              game.send({ type: 'end' });
              endConfirm = false;
            "
          >
            結束本場
          </button>
        </div>
      </section>
    </div>
  </div>
</template>
