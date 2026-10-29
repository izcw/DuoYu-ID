import type { ObjectDirective } from 'vue'

const cleanup = new WeakMap<HTMLElement, () => void>()

// Keep security fields out of the initial credential autofill scan. Unlock on
// focus for keyboard, touch and paste; never clear manually entered values.
export const vManualEntry: ObjectDirective<HTMLElement> = {
  mounted(element) {
    const input = element.querySelector('input')
    if (!input) return
    input.classList.add('dy-manual-entry')
    input.readOnly = true
    const focus = () => { input.readOnly = false }
    const blur = () => { input.readOnly = true }
    const autofill = (event: AnimationEvent) => {
      if (event.animationName !== 'dy-reject-autofill') return
      input.value = ''
      input.dispatchEvent(new Event('input', { bubbles: true }))
    }
    input.addEventListener('focus', focus)
    input.addEventListener('blur', blur)
    input.addEventListener('animationstart', autofill)
    cleanup.set(element, () => {
      input.removeEventListener('focus', focus)
      input.removeEventListener('blur', blur)
      input.removeEventListener('animationstart', autofill)
    })
  },
  unmounted(element) { cleanup.get(element)?.(); cleanup.delete(element) }
}
