<template>
  <div class="min-h-screen bg-white dark:bg-dark-950">
    <iframe
      :src="src"
      :title="title"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  src: string
  title: string
}>()
</script>
