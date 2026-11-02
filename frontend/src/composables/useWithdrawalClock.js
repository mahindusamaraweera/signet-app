import { ref, onMounted, onBeforeUnmount } from 'vue'
import { deadlineMillis } from '../utils/withdrawalTime'

export function useWithdrawalClock() {
  let anchor = Date.now(), elapsed = performance.now(), interval
  const now = ref(anchor)
  const tick = () => { now.value = anchor + performance.now() - elapsed }
  function sync(serverTime) {
    const time = deadlineMillis(serverTime)
    if (time === null) return
    anchor = time; elapsed = performance.now(); tick()
  }
  onMounted(() => { interval = setInterval(tick, 1000) })
  onBeforeUnmount(() => clearInterval(interval))
  return { now, sync }
}
