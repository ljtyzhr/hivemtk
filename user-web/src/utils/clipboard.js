// navigator.clipboard is undefined on insecure origins and rejects without document
// focus/permission, and execCommand('copy') returns false instead of throwing, so the
// only trustworthy signal is the boolean below — callers must not toast success on it.
function legacyCopy(text) {
  try {
    const area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.top = '-9999px'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    area.setSelectionRange(0, text.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(area)
    return ok === true
  } catch {
    return false
  }
}

export async function copyToClipboard(text) {
  const value = text == null ? '' : String(text)
  if (!value) return false
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return true
    } catch {
      return legacyCopy(value)
    }
  }
  return legacyCopy(value)
}
