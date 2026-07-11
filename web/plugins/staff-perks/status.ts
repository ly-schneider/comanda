import type { VolunteerCampaignStatus } from "@/types/volunteer"

export const STATUS_LABEL: Record<VolunteerCampaignStatus, string> = {
  draft: "Entwurf",
  active: "Aktiv",
  ended: "Beendet",
}

export const STATUS_CLASS: Record<VolunteerCampaignStatus, string> = {
  draft: "bg-amber-100 text-amber-900",
  active: "bg-emerald-100 text-emerald-900",
  ended: "bg-muted text-muted-foreground",
}
