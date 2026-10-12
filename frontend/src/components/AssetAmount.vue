<script setup>
import { assetDisplay } from '@/utils/assetDisplay'
defineProps({ value: { type: [String, Number], default: null }, asset: { type: String, default: 'USDT' } })
</script>
<template><span class="asset-amount" :title="value == null ? 'Balance unavailable' : `${value} ${asset}`">{{ assetDisplay(value, asset) }} <small>{{ asset }}</small></span></template>
