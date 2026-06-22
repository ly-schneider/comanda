-- The host resolver now requires verified_at before a hostname routes to its
-- tenant, so that a BYO custom domain only resolves once its ownership has been
-- proven (CNAME + TXT). Platform-assigned hosts (tenant subdomains, localhost,
-- the default host) are trusted by construction and must keep resolving. No BYO
-- custom domains exist yet, so every currently-unverified row is platform
-- assigned — mark them verified to preserve existing routing.
UPDATE tenant_domain SET verified_at = NOW() WHERE verified_at IS NULL;
