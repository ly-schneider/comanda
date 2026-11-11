"use client"

import { useEffect, useState } from "react"
import { cn } from "@/lib/utils"

export function LegalToc({ sections }: { sections: { id: string; title: string }[] }) {
  const [active, setActive] = useState(sections[0]?.id)

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible[0]) setActive(visible[0].target.id)
      },
      { rootMargin: "-80px 0px -65% 0px" }
    )
    for (const s of sections) {
      const el = document.getElementById(s.id)
      if (el) observer.observe(el)
    }
    return () => observer.disconnect()
  }, [sections])

  return (
    <ol>
      {sections.map((s, i) => (
        <li key={s.id}>
          <a
            href={`#${s.id}`}
            className={cn(
              "block border-l py-[7px] pl-3.5 text-[13px] leading-5 transition-colors",
              active === s.id
                ? "border-foreground text-foreground border-l-2 pl-[13px] font-medium"
                : "border-border text-muted-foreground hover:text-foreground"
            )}
          >
            {i + 1}. {s.title}
          </a>
        </li>
      ))}
    </ol>
  )
}
