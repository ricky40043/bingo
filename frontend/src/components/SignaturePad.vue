<script setup lang="ts">
import { onMounted, ref } from "vue";
const props = defineProps<{ question: string; busy: boolean }>();
const emit = defineEmits<{
  close: [];
  submit: [value: { name: string; ink: string }];
}>();
const canvas = ref<HTMLCanvasElement>(),
  hasInk = ref(false);
let drawing = false;
function point(e: PointerEvent) {
  const r = canvas.value!.getBoundingClientRect();
  return [
    ((e.clientX - r.left) * 640) / r.width,
    ((e.clientY - r.top) * 240) / r.height,
  ];
}
function down(e: PointerEvent) {
  if (props.busy) return;
  drawing = true;
  canvas.value!.setPointerCapture(e.pointerId);
  const c = canvas.value!.getContext("2d")!;
  const [x, y] = point(e);
  c.beginPath();
  c.moveTo(x, y);
  c.lineTo(x + 0.2, y + 0.2);
  c.stroke();
  hasInk.value = true;
}
function move(e: PointerEvent) {
  if (!drawing) return;
  const c = canvas.value!.getContext("2d")!;
  const [x, y] = point(e);
  c.lineTo(x, y);
  c.stroke();
}
function clear() {
  canvas.value!.getContext("2d")!.clearRect(0, 0, 640, 240);
  hasInk.value = false;
}
onMounted(() => {
  const c = canvas.value!.getContext("2d")!;
  c.lineWidth = 3;
  c.lineCap = "round";
  c.lineJoin = "round";
  c.strokeStyle = "#324837";
});
function submit() {
  if (!hasInk.value || props.busy) return;
  emit("submit", {
    name: "",
    ink: canvas.value!.toDataURL("image/png"),
  });
}
</script>
<template>
  <div class="overlay" @click.self="!busy && emit('close')">
    <section
      class="signature-dialog"
      role="dialog"
      aria-modal="true"
      aria-labelledby="signature-title"
      @keydown.esc="!busy && emit('close')"
    >
      <button
        class="close-button"
        aria-label="關閉簽名"
        :disabled="busy"
        @click="emit('close')"
      >
        ×
      </button>
      <p class="eyebrow">NICE TO MEET YOU</p>
      <h2 id="signature-title">這一格，我來簽！</h2>
      <p class="sign-question">{{ question }}</p>
      <p class="muted">請把手機交給符合這個描述的朋友，讓他直接在下面簽名。</p>
      <form @submit.prevent="submit">
        <div class="label-row">
          <span>手寫簽名 <small>（必填）</small></span
          ><button
            type="button"
            class="text-button"
            :disabled="busy"
            @click="clear"
          >
            清除重寫
          </button>
        </div>
        <canvas
          ref="canvas"
          width="640"
          height="240"
          aria-label="手寫簽名區"
          @pointerdown="down"
          @pointermove="move"
          @pointerup="drawing = false"
          @pointercancel="drawing = false"
        /><button class="primary full" :disabled="busy || !hasInk">
          {{ busy ? "正在送出…" : "確認簽名 ✓" }}
        </button>
      </form>
    </section>
  </div>
</template>
