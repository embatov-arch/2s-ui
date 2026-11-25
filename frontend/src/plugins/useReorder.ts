import { ref } from 'vue'

// Apply to a live list or to a transaction's detached copy.
export function moveItem<T>(arr: T[], index: number, dir: number): void {
  const to = index + dir
  if (!Number.isInteger(index) || !Number.isInteger(to) || index < 0 || index >= arr.length || to < 0 || to >= arr.length || to === index) return
  const item = arr[index]!
  arr.splice(index, 1)
  arr.splice(to, 0, item)
}

// Drag-and-drop plus move-up/down for the rule lists on Rules and Dns.
//
// Takes a getter and only ever splices. Both callers pass a readonly computed
// (`rules`, `dnsRules`) that resolves to the array hanging off the config
// object, so reassigning would fail at runtime — Vue logs "Write operation
// failed: computed value is readonly" and the click silently does nothing. A
// Ref<any[]> parameter would accept those computeds at compile time and hide
// exactly that, which is why this is a getter.
export function useReorder(list: () => any[]) {
  const draggedItemIndex = ref<number | null>(null)

  const onDragStart = (index: number) => { draggedItemIndex.value = index }

  const onDrop = (index: number) => {
    if (draggedItemIndex.value !== null) {
      moveItem(list(), draggedItemIndex.value, index - draggedItemIndex.value)
      draggedItemIndex.value = null
    }
  }

  const move = (index: number, dir: number) => moveItem(list(), index, dir)

  return { onDragStart, onDrop, move }
}
