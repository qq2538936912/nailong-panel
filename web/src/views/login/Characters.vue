<script setup lang="ts">
import { computed } from "vue";

export type CharacterMood =
  | "idle"
  | "typing"
  | "password"
  | "peek"
  | "success"
  | "error";

const props = defineProps<{
  mouseX: number;
  mouseY: number;
  mood: CharacterMood;
}>();

const px = computed(() => props.mouseX * 4);
const py = computed(() => props.mouseY * 3);

const coverEyes = computed(() => props.mood === "password");
const lookAway = computed(() => props.mood === "peek");
const smile = computed(() => props.mood === "success");
const sad = computed(() => props.mood === "error");

const bodyTilt = computed(() => (lookAway.value ? -10 : props.mouseX * 4));
const headTilt = computed(() => (lookAway.value ? -18 : props.mouseX * 6));
const headShift = computed(() => (lookAway.value ? -16 : 0));

const bodyTransform = computed(
  () => `translate(210, 248) rotate(${bodyTilt.value * 0.35}, 0, 40)`,
);
const headTransform = computed(
  () =>
    `translate(${headShift.value}, ${lookAway.value ? -6 : 0}) rotate(${headTilt.value * 0.35}, 0, -18)`,
);
</script>

<template>
  <svg
    viewBox="0 0 420 410"
    width="100%"
    height="100%"
    style="overflow: visible"
    role="img"
    aria-label="奶龙"
  >
    <g class="char-body" :transform="bodyTransform">
      <ellipse cx="92" cy="46" rx="28" ry="16" fill="#F5C84B" transform="rotate(22 92 46)" />
      <ellipse cx="0" cy="58" rx="78" ry="62" fill="#FFE14A" />
      <ellipse cx="-62" cy="78" rx="18" ry="14" fill="#F6C63A" />
      <ellipse cx="62" cy="78" rx="18" ry="14" fill="#F6C63A" />
      <ellipse cx="-28" cy="108" rx="16" ry="12" fill="#F0B72C" />
      <ellipse cx="28" cy="108" rx="16" ry="12" fill="#F0B72C" />

      <g class="char-face" :transform="headTransform">
        <ellipse cx="0" cy="-38" rx="86" ry="78" fill="#FFE14A" />
        <ellipse cx="-70" cy="-58" rx="16" ry="12" fill="#F6C63A" transform="rotate(-28 -70 -58)" />
        <ellipse cx="70" cy="-58" rx="16" ry="12" fill="#F6C63A" transform="rotate(28 70 -58)" />

        <template v-if="coverEyes">
          <ellipse cx="-28" cy="-8" rx="22" ry="16" fill="#F4C43A" transform="rotate(-12 -28 -8)" />
          <ellipse cx="28" cy="-8" rx="22" ry="16" fill="#F4C43A" transform="rotate(12 28 -8)" />
        </template>
        <template v-else>
          <ellipse cx="-28" cy="-18" rx="16" ry="20" fill="#1A1A1A" />
          <ellipse cx="28" cy="-18" rx="16" ry="20" fill="#1A1A1A" />
          <circle :cx="-22 + px * 0.35" :cy="-24 + py * 0.3" r="4.5" fill="#fff" />
          <circle :cx="34 + px * 0.35" :cy="-24 + py * 0.3" r="4.5" fill="#fff" />
        </template>

        <ellipse cx="-10" cy="4" rx="3.2" ry="2.4" fill="#C9892A" />
        <ellipse cx="10" cy="4" rx="3.2" ry="2.4" fill="#C9892A" />

        <path
          v-if="smile"
          d="M -22,22 Q 0,42 22,22"
          fill="none"
          stroke="#1A1A1A"
          stroke-width="6"
          stroke-linecap="round"
        />
        <path
          v-else-if="sad"
          d="M -20,34 Q 0,18 20,34"
          fill="none"
          stroke="#1A1A1A"
          stroke-width="6"
          stroke-linecap="round"
        />
        <path
          v-else
          d="M -16,24 Q 0,32 16,24"
          fill="none"
          stroke="#1A1A1A"
          stroke-width="6"
          stroke-linecap="round"
        />
      </g>
    </g>
  </svg>
</template>

<style scoped>
:deep(.char-body) {
  transition: transform 0.4s cubic-bezier(0.25, 0.46, 0.45, 0.94);
}
:deep(.char-face) {
  transition: transform 0.25s ease-out;
}
</style>
