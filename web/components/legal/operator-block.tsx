import { PLATFORM_OPERATOR } from "@/lib/legal"

// The platform operator's fixed address block, shared by the privacy policy and
// the terms so the operator's details can never drift between the two pages.
export function OperatorBlock() {
  const lines = [
    PLATFORM_OPERATOR.legalName,
    PLATFORM_OPERATOR.owner,
    ...PLATFORM_OPERATOR.addressLines,
    PLATFORM_OPERATOR.country,
  ]
  return (
    <>
      <p className="mt-2">
        {lines.map((line, i) => (
          <span key={line}>
            {i > 0 && <br />}
            {line}
          </span>
        ))}
      </p>
      <p className="mt-2">E-Mail: {PLATFORM_OPERATOR.email}</p>
    </>
  )
}
