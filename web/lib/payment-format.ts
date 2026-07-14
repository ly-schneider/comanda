// Labels for the backend order-payment enum (order.payments[].method).
export const PAYMENT_METHOD_LABELS: Record<string, string> = {
  CASH: "Bargeld",
  CARD: "Karte",
  TWINT: "TWINT",
  GRATIS_GUEST: "Gratis (Gast)",
  GRATIS_VIP: "Gratis (VIP)",
  GRATIS_STAFF: "Gratis (Mitarbeiter)",
  GRATIS_PLUGIN: "Gratis",
}

export function paymentMethodLabel(method: string): string {
  return PAYMENT_METHOD_LABELS[method] ?? method
}
