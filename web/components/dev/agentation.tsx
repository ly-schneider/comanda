"use client"

import { Agentation } from "agentation"
import { useEffect, useState } from "react"

const AGENTATION_ENDPOINT = "http://localhost:4747"

/* `navigator.clipboard` only exists in a secure context, so it is absent when dev
   is reached over plain HTTP from another device (POS/station testing on a LAN
   IP). The library swallows the failure, so the copy button reports success
   while the clipboard stays empty. Runs inside the click handler, so
   execCommand still has user activation. */
function copyToClipboard(text: string) {
  const legacyCopy = () => {
    const textarea = document.createElement("textarea")
    textarea.value = text
    textarea.setAttribute("readonly", "")
    textarea.style.position = "fixed"
    textarea.style.top = "0"
    textarea.style.opacity = "0"
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand("copy")
    textarea.remove()
  }

  if (!navigator.clipboard?.writeText) {
    legacyCopy()
    return
  }

  navigator.clipboard.writeText(text).catch(legacyCopy)
}

/* Portals into document.body, so mounting is deferred to the client. */
export function DevAnnotations() {
  const [mounted, setMounted] = useState(false)

  useEffect(() => setMounted(true), [])

  if (!mounted) return null

  return <Agentation endpoint={AGENTATION_ENDPOINT} copyToClipboard={false} onCopy={copyToClipboard} />
}
