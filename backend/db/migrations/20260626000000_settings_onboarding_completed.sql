-- Records when a vendor finishes (or dismisses) the onboarding wizard. NULL =
-- not yet completed; per-step progress is derived from brand/payments/domain data.
ALTER TABLE settings
  ADD COLUMN onboarding_completed_at TIMESTAMPTZ;
