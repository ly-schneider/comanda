// Manual-QA checklist for /platform-admin/qa. Stored results reference
// `QaTest.key` — never rename a key once recorded; add new tests instead.
// Sections are ordered so each produces the artifacts the next one needs.

export type QaStatus = "success" | "error"

export type QaTest = {
  key: string
  title: string
  action: string
  expected: string
}

export type QaSection = {
  key: string
  title: string
  intro?: string
  haveReady: string[]
  tests: QaTest[]
}

export const QA_SECTIONS: QaSection[] = [
  {
    key: "smoke",
    title: "0 · Plattform-Smoke & Zugriff",
    haveReady: [
      "Staging-Deployment erreichbar (API + Web)",
      "Platform-Admin-Benutzer (is_platform_admin gesetzt)",
      "Normaler Benutzer ohne Admin-Flag",
      "Inkognito-Fenster",
    ],
    tests: [
      {
        key: "smoke.health",
        title: "Health-Endpunkte antworten",
        action: "GET /ping und GET /health auf der API aufrufen (curl oder Browser).",
        expected: "/ping antwortet 200. /health antwortet 200 mit Status ok und Versionsangabe.",
      },
      {
        key: "smoke.platform-admin-gate",
        title: "/platform-admin blockiert Nicht-Admins",
        action:
          "Als normaler Benutzer auf dem Plattform-Host einloggen und /platform-admin öffnen; zusätzlich GET /api/v1/platform/tenants direkt aufrufen.",
        expected: "Die UI leitet auf die Startseite um (PlatformAdminGuard); der API-Call antwortet 403.",
      },
      {
        key: "smoke.platform-admin-access",
        title: "Platform-Admin erreicht das Control-Plane",
        action: "Als Platform-Admin /platform-admin öffnen.",
        expected: "Die Tenant-Liste lädt; «Tenant erstellen» ist sichtbar.",
      },
    ],
  },
  {
    key: "provisioning",
    title: "1 · Tenant-Provisionierung (Platform-Admin)",
    haveReady: [
      "Freier Test-Slug (z. B. qa-<datum>)",
      "E-Mail-Postfach für die Inhaber-Einladung",
      "Frisches Inkognito-Fenster für die Annahme",
    ],
    tests: [
      {
        key: "provisioning.create-tenant",
        title: "Tenant erstellen",
        action:
          "Unter /platform-admin/tenants/new Slug + Name eingeben und erstellen; danach https://<slug>.<basisdomain>/ öffnen.",
        expected:
          "Tenant erscheint aktiv in der Liste; die Subdomain liefert den (leeren) Shop mit neutralem Branding aus.",
      },
      {
        key: "provisioning.slug-validation",
        title: "Ungültige Slugs werden abgelehnt",
        action:
          "Slugs mit Grossbuchstaben, führendem/abschliessendem Bindestrich sowie reservierte Slugs (www, api, admin, platform …) versuchen.",
        expected: "Formular bzw. API lehnen mit klarer Fehlermeldung ab (400); kein Tenant entsteht.",
      },
      {
        key: "provisioning.duplicate-slug",
        title: "Doppelter Slug wird abgelehnt",
        action: "Einen bereits vergebenen Slug erneut anlegen.",
        expected: "409 «Slug bereits vergeben»; kein zweiter Tenant.",
      },
      {
        key: "provisioning.seeded-rows",
        title: "Neuer Tenant ist korrekt geseedet",
        action:
          "Nach dem Erstellen die Tenant-Detailseite öffnen und im Account-Hub des Tenants Branding/Domains prüfen.",
        expected:
          "Settings-Zeile existiert (neutrale Defaults); die Plattform-Subdomain ist als verifizierte Standard-Domain gesetzt und nicht löschbar.",
      },
      {
        key: "provisioning.suspend-reactivate",
        title: "Sperren und Reaktivieren",
        action: "Tenant auf der Detailseite sperren, Storefront/Admin/API des Tenants aufrufen, danach reaktivieren.",
        expected:
          "Gesperrt: Shop, Admin und System-Status blockieren (Splash bzw. 403). Nach Reaktivierung funktioniert alles wieder.",
      },
      {
        key: "provisioning.owner-invite-email",
        title: "Inhaber-Einladung wird versendet",
        action: "Auf der Tenant-Detailseite eine Inhaber-Einladung an ein Testpostfach senden.",
        expected:
          "E-Mail kommt an: deutscher Text, Rolle «Inhaber», Ablaufzeit (Europe/Zurich), Link zeigt auf den Tenant-Host; Staging-Umgebung hat [Staging]-Präfix im Betreff.",
      },
      {
        key: "provisioning.owner-invite-accept",
        title: "Inhaber-Einladung annehmen",
        action: "Im Inkognito-Fenster den Einladungslink öffnen, Namen erfassen und annehmen; anschliessend einloggen.",
        expected:
          "Annahme führt zur Plattform-Anmeldung (?next=/admin); nach dem Login ist der Benutzer Owner der Organisation und erreicht /admin des Tenants.",
      },
    ],
  },
  {
    key: "auth",
    title: "2 · Authentifizierung & Session",
    haveReady: ["Testpostfach für OTP-Codes", "Google-Konto", "Erreichbare Tenant-Subdomain"],
    tests: [
      {
        key: "auth.otp-login",
        title: "E-Mail-OTP-Anmeldung",
        action: "/login öffnen, E-Mail eingeben, «Code senden», 6-stelligen Code aus dem Postfach eintragen.",
        expected:
          "OTP-E-Mail kommt zeitnah an; nach korrektem Code ist die Session aktiv und es wird auf / (bzw. ?next=) weitergeleitet.",
      },
      {
        key: "auth.otp-errors",
        title: "OTP-Fehlerfälle",
        action: "Falschen Code eingeben; «Code erneut senden» nutzen; über «Andere E-Mail» die Adresse wechseln.",
        expected:
          "Falscher Code wird abgelehnt (Fehlermeldung, kein Login); erneuter Versand liefert neuen gültigen Code; Adresswechsel setzt den Flow sauber zurück.",
      },
      {
        key: "auth.google-platform",
        title: "Google-Login auf dem Plattform-Host",
        action: "Auf comanda.leys.ch «Mit Google anmelden» durchspielen.",
        expected: "OAuth-Flow läuft ohne Fehler; Session aktiv, Profil zeigt Google-E-Mail.",
      },
      {
        key: "auth.google-subdomain",
        title: "Google-Login auf Tenant-Subdomain",
        action: "Auf <slug>.<basisdomain> den Google-Login durchspielen (oAuthProxy-Pfad).",
        expected: "Kein state_mismatch; Redirect landet wieder auf der Subdomain, Session aktiv.",
      },
      {
        key: "auth.profile",
        title: "Profil: Name, Abmelden, Konto löschen",
        action:
          "/profile öffnen: Namen ändern und speichern; abmelden; mit Wegwerf-Konto «Konto löschen» inkl. Bestätigungsdialog durchspielen.",
        expected:
          "Name persistiert; Abmelden leert die Session und leitet auf /; Löschung fragt nach, entfernt das Konto und beendet die Session.",
      },
      {
        key: "auth.gating",
        title: "Zugriffs-Gates ohne Mitgliedschaft",
        action:
          "Als Kunde ohne Org-Mitgliedschaft /admin auf dem Tenant-Host sowie /account auf dem Plattform-Host öffnen.",
        expected: "/admin verweigert (Redirect/403); /account verlangt eine aktive Organisation.",
      },
    ],
  },
  {
    key: "account-hub",
    title: "3 · Account-Hub & Onboarding",
    haveReady: ["Owner-Login des QA-Tenants", "Optional: Benutzer mit Zugriff auf zwei Tenants (Switcher-Test)"],
    tests: [
      {
        key: "account-hub.entry",
        title: "Hub-Einstieg und Branding",
        action: "comanda.leys.ch/account öffnen.",
        expected:
          "Redirect auf /t/<slug>; der Hub rendert mit dem Branding des Tenants (Name, Logo, Farben), der Plattform-Host bleibt neutral.",
      },
      {
        key: "account-hub.switcher",
        title: "Tenant-Wechsel im Hub",
        action: "Im Header-Dropdown auf den zweiten Tenant wechseln.",
        expected:
          "URL wechselt auf /t/<slug2>, Branding und Daten gehören vollständig zum zweiten Tenant; kein Datenleck des ersten.",
      },
      {
        key: "account-hub.onboarding-derived",
        title: "Onboarding-Checkliste zeigt abgeleiteten Stand",
        action: "Beim frischen Tenant die Übersicht /t/<slug> ansehen.",
        expected:
          "Checkliste zeigt «Konto erstellt» erledigt, Marke/Zahlungen/Domain offen inkl. fehlender Felder; Fortschritt («x von y») stimmt.",
      },
      {
        key: "account-hub.onboarding-progress",
        title: "Schritte kippen mit den Daten",
        action: "Nach Abschluss der Abschnitte Branding (4), Payrexx (5) und Domain (7) die Checkliste erneut prüfen.",
        expected:
          "Jeder Schritt springt automatisch auf erledigt, sobald die Daten vorhanden sind (kein manuelles Abhaken nötig).",
      },
      {
        key: "account-hub.onboarding-complete",
        title: "Onboarding abschliessen",
        action: "«Onboarding abschliessen» klicken und die Seite neu laden.",
        expected: "Karte verschwindet dauerhaft (onboarding_completed_at gesetzt); Einstellungen bleiben editierbar.",
      },
    ],
  },
  {
    key: "branding",
    title: "4 · Branding & Rechtliches",
    haveReady: ["Logo als PNG/WebP < 5 MB + Favicon", "Hex-Farbwerte der Marke", "Firmenname, Adresse, Kontakt-E-Mail"],
    tests: [
      {
        key: "branding.save-fields",
        title: "Branding-Felder speichern",
        action:
          "Unter /t/<slug>/branding Anzeigename, Währung, Farben, Schrift, Radius, Kontakt und rechtliche Angaben setzen und speichern.",
        expected: "«Gespeichert»-Bestätigung; alle Werte überleben einen Reload.",
      },
      {
        key: "branding.validation",
        title: "Ungültige Eingaben werden abgelehnt",
        action: "Ungültigen Hex-Wert (z. B. #12) und ungültige Währung (z. B. chf1) speichern.",
        expected: "Client/Server lehnen mit verständlicher Fehlermeldung ab; nichts wird gespeichert.",
      },
      {
        key: "branding.logo-upload",
        title: "Logo & Favicon hochladen",
        action: "Logo und Favicon hochladen; danach Shop und Admin des Tenants öffnen.",
        expected: "Vorschau erscheint; Logo/Favicon werden im Storefront/Admin ausgeliefert (R2-URL erreichbar).",
      },
      {
        key: "branding.logo-delete",
        title: "Logo & Favicon entfernen",
        action: "Beide über «Entfernen» löschen.",
        expected: "Vorschau verschwindet, Shop fällt auf neutralen Zustand zurück; kein Fehler.",
      },
      {
        key: "branding.propagation",
        title: "Branding wirkt überall",
        action:
          "Storefront (Farben, Name), Nutzungsbedingungen/Datenschutz (dynamische Felder) und eine Quittungs-E-Mail prüfen.",
        expected: "Alle Oberflächen zeigen die Tenant-Marke; Rechtstexte enthalten Firmenname/Adresse/E-Mail.",
      },
      {
        key: "branding.platform-neutral",
        title: "Plattform-Host bleibt neutral",
        action: "Nach dem Branding comanda.leys.ch (Login, Platform-Admin) ansehen.",
        expected: "Kein Tenant-Branding auf dem Plattform-Host.",
      },
    ],
  },
  {
    key: "payrexx",
    title: "5 · Payrexx-Anbindung",
    haveReady: ["Payrexx-Testinstanz + API-Key", "Zugang zum Payrexx-Dashboard (Webhook anlegen)"],
    tests: [
      {
        key: "payrexx.invalid-credentials",
        title: "Falsche Zugangsdaten",
        action: "Unter /t/<slug>/payrexx bewusst falsche Instanz/API-Key verbinden.",
        expected: "Klarer Fehler («Zugangsdaten konnten nicht überprüft werden»); Status bleibt unverbunden.",
      },
      {
        key: "payrexx.connect",
        title: "API-Zugang verbinden",
        action: "Korrekte Instanz + API-Key eintragen und verbinden.",
        expected:
          "Live-Validierung gegen Payrexx gelingt; Status «Verbunden», API-Key-Feld leert sich (write-only), Schritt 2 wird aktiv.",
      },
      {
        key: "payrexx.test-connection",
        title: "Verbindung erneut testen",
        action: "«Erneut testen» klicken.",
        expected: "«Verbindung erfolgreich» mit gespeicherten Credentials.",
      },
      {
        key: "payrexx.webhook-setup",
        title: "Webhook einrichten",
        action:
          "Webhook-URL kopieren, im Payrexx-Dashboard einen Transaction-Webhook (JSON, Retry aktiv) anlegen und das generierte Secret speichern.",
        expected: "«Webhook-Secret gespeichert»; Status wechselt auf vollständig konfiguriert.",
      },
      {
        key: "payrexx.webhook-received",
        title: "Webhook-Empfang bestätigt",
        action: "Eine Testtransaktion in Payrexx auslösen (oder Abschnitt 11 abwarten).",
        expected: "«Letzter Webhook empfangen» zeigt einen aktuellen Zeitstempel.",
      },
      {
        key: "payrexx.update-credentials",
        title: "Zugangsdaten aktualisieren",
        action: "Über «Aktualisieren» einen neuen API-Key setzen.",
        expected: "Neue Credentials werden validiert und gespeichert; alter Key ist nicht mehr sichtbar.",
      },
    ],
  },
  {
    key: "sumup-settings",
    title: "6 · SumUp-Einstellungen (Account-Hub)",
    haveReady: ["SumUp-Händlerkonto (me.sumup.com) mit Merchant-Code"],
    tests: [
      {
        key: "sumup-settings.save-code",
        title: "Merchant-Code hinterlegen",
        action: "Unter /t/<slug>/sumup den SumUp-Händler-Code eintragen und speichern.",
        expected: "«Gespeichert» + Badges «Aktiv»/«Hinterlegt»; Wert überlebt Reload.",
      },
      {
        key: "sumup-settings.clear-code",
        title: "Kontoprüfung deaktivieren",
        action: "Feld leeren und speichern.",
        expected:
          "Badge verschwindet; Kassen warnen künftig nur noch statt zu blockieren (Verhalten wird in Abschnitt 13 verifiziert).",
      },
    ],
  },
  {
    key: "domains",
    title: "7 · Eigene Domains (BYO)",
    intro:
      "Früh starten: DNS-Propagation und Zertifikatsausstellung brauchen Zeit — Rest der Liste parallel weitertesten.",
    haveReady: [
      "Eigene Testdomain mit DNS-Zugriff (CNAME + TXT setzbar)",
      "Staging-Cluster mit cert-manager/Traefik aktiv",
    ],
    tests: [
      {
        key: "domains.add",
        title: "Domain hinzufügen",
        action: "Unter /t/<slug>/domains die Testdomain erfassen.",
        expected:
          "Domain erscheint unverifiziert; DNS-Anleitung mit CNAME-Ziel und TXT (_comanda-verify + Token) inkl. Copy-Buttons.",
      },
      {
        key: "domains.validation",
        title: "Ungültige Hostnames werden abgelehnt",
        action:
          "Ungültige FQDN, eine Plattform-Subdomain (x.comanda.leys.ch) und eine bereits registrierte Domain versuchen.",
        expected: "Jeweils 400/409 mit klarer Meldung; keine Zeile entsteht.",
      },
      {
        key: "domains.verify-early",
        title: "Prüfung vor DNS-Setup schlägt sauber fehl",
        action: "«Jetzt prüfen» klicken, bevor die DNS-Records existieren.",
        expected: "Fehlschlag ohne Absturz; Domain bleibt unverifiziert, erneuter Versuch möglich.",
      },
      {
        key: "domains.verify",
        title: "Verifizierung nach DNS-Setup",
        action: "CNAME + TXT beim DNS-Provider setzen, Propagation abwarten, «Jetzt prüfen».",
        expected: "Domain wird «Verifiziert» (auch der Hintergrund-Sweep verifiziert automatisch).",
      },
      {
        key: "domains.certificate",
        title: "TLS-Zertifikat wird ausgestellt",
        action: "Zertifikatsstatus beobachten (pending → aktiv), ggf. kubectl get certificate.",
        expected: "Status wechselt auf «Zertifikat aktiv»; Buttons «Storefront öffnen»/«Admin öffnen» erscheinen.",
      },
      {
        key: "domains.set-primary",
        title: "Als Standard-Domain setzen",
        action: "Bei der verifizierten Domain «Als Standard festlegen» klicken.",
        expected:
          "Standard-Badge wandert; nur verifizierte Domains sind wählbar; Kundenlinks (Quittung) nutzen fortan diese Domain.",
      },
      {
        key: "domains.serves-both",
        title: "BYO-Domain liefert Shop und Admin",
        action: "https://<domain>/ und https://<domain>/admin öffnen.",
        expected: "Beide Oberflächen laden über HTTPS mit gültigem Zertifikat und Tenant-Branding.",
      },
      {
        key: "domains.session",
        title: "Login auf der BYO-Domain",
        action: "Auf der BYO-Domain einloggen und /admin sowie /profile aufrufen.",
        expected: "Keine Redirect-Schleife (Cookie/Proto-Fix); Session bleibt über Seitenwechsel stabil.",
      },
      {
        key: "domains.google-oauth",
        title: "Google-Login auf der BYO-Domain",
        action: "Google-OAuth auf der BYO-Domain durchspielen.",
        expected: "Kein state_mismatch; Redirect landet auf der BYO-Domain.",
      },
      {
        key: "domains.delete",
        title: "Domain löschen / Plattform-Domain geschützt",
        action: "Die BYO-Domain löschen (Bestätigung); danach versuchen, die geseedete Plattform-Subdomain zu löschen.",
        expected:
          "BYO-Domain verschwindet (nächste verifizierte wird Standard); Plattform-Domain lässt sich nicht löschen (400 platform_domain).",
      },
    ],
  },
  {
    key: "invites",
    title: "8 · Team-Einladungen & Rollen",
    haveReady: ["3–4 Testpostfächer (Plus-Adressierung reicht)", "Inkognito-Fenster pro Annahme"],
    tests: [
      {
        key: "invites.hub-admin",
        title: "Hub: Administrator einladen",
        action: "Unter /t/<slug>/invites eine E-Mail einladen, Link im Postfach öffnen, annehmen, einloggen.",
        expected:
          "Invite steht auf PENDING → ACCEPTED; neues Mitglied hat Rolle admin und erreicht Account-Hub und /admin.",
      },
      {
        key: "invites.admin-roles",
        title: "/admin: Rollen staff/catalog/app_admin einladen",
        action:
          "Unter /admin/invites je eine Einladung als Mitarbeiter, Sortiment und App-Admin versenden und annehmen.",
        expected: "Jede Annahme ergibt die korrekte Stufe; Betreff nennt die Rolle (z. B. «Einladung als Sortiment»).",
      },
      {
        key: "invites.tier-gating",
        title: "Stufen-Gating in /admin",
        action: "Mit catalog-, staff- und app_admin-Konten die /admin-Bereiche durchklicken.",
        expected:
          "catalog: nur Produkte/Kategorien/Bundles. staff: zusätzlich Bestellungen/POS/Stationen/Geräte/Inventar/Jetons/Mitarbeiter-Gutscheine, aber keine Benutzer/Einladungen/Einstellungen. app_admin: alles in /admin.",
      },
      {
        key: "invites.hub-gating",
        title: "Hub bleibt Verwaltungsrollen vorbehalten",
        action: "Mit dem staff-Konto comanda.leys.ch/account öffnen.",
        expected: "Keine Verwaltungsansicht für den Tenant (kein Owner/Admin) — Zugriff wird verweigert.",
      },
      {
        key: "invites.revoke",
        title: "Einladung zurückziehen",
        action: "Eine pendente Einladung löschen und danach den Link öffnen.",
        expected: "Link ist ungültig; saubere Fehlermeldung statt Annahme.",
      },
      {
        key: "invites.expired",
        title: "Abgelaufene Einladung",
        action: "Einen abgelaufenen Invite-Link öffnen (Ablauf per DB oder 7 Tage warten).",
        expected: "Verständliche «abgelaufen»-Meldung; keine Mitgliedschaft entsteht.",
      },
    ],
  },
  {
    key: "catalog",
    title: "9 · Sortiment (Produkte, Kategorien, Bundles, Jetons)",
    haveReady: ["Produktbilder", "Entscheid für den POS-Modus (QR_CODE/JETON/HYBRID)"],
    tests: [
      {
        key: "catalog.categories",
        title: "Kategorien verwalten",
        action:
          "Unter /admin/categories Kategorien anlegen (Positionen 0/10/5), Namen inline ändern, Aktiv-Schalter toggeln, eine löschen.",
        expected:
          "Sortierung folgt der Position; inaktive Kategorien verschwinden aus dem Shop; Löschung entkoppelt Produkte sauber.",
      },
      {
        key: "catalog.product-create",
        title: "Einfaches Produkt anlegen",
        action: "Unter /admin/products ein Produkt mit Name, Preis, Beschreibung, Kategorie und Bild anlegen.",
        expected: "Produkt erscheint in Liste und Shop; CHF-Preis korrekt auf 2 Stellen normalisiert.",
      },
      {
        key: "catalog.product-deposit",
        title: "Produkt mit Depot",
        action: "Ein Produkt mit Depot (z. B. CHF 2.00) anlegen.",
        expected: "Depot wird gespeichert und im Shop als «+ CHF 2.00 Depot» angezeigt.",
      },
      {
        key: "catalog.product-edit",
        title: "Produkt bearbeiten/archivieren/löschen",
        action: "Name/Preis/Kategorie ändern; «Aktiv» deaktivieren; ein Wegwerf-Produkt löschen.",
        expected:
          "Änderungen persistieren; inaktives Produkt zeigt im Shop «Nicht aktiv»-Overlay und ist nicht bestellbar; Löschung entfernt es.",
      },
      {
        key: "catalog.product-image",
        title: "Produktbild hochladen und löschen",
        action: "Bild setzen, prüfen, wieder entfernen.",
        expected: "Bild erscheint in Liste + Shop; nach Löschung Platzhalter, kein Fehler.",
      },
      {
        key: "catalog.inventory",
        title: "Lagerbestand führen",
        action: "Anfangsbestand setzen, per +/- korrigieren; /admin/inventory für das Produkt öffnen.",
        expected: "Ledger zeigt opening_balance und manual_adjust mit Delta und Verursacher; Bestand nie negativ.",
      },
      {
        key: "catalog.jetons",
        title: "Jetons verwalten",
        action:
          "Unter /admin/jetons Jeton mit Palette + Hex-Farbe anlegen, einem Produkt zuweisen, Löschung eines verwendeten Jetons versuchen.",
        expected: "Jeton mit Farb-Badge; Zuweisung sichtbar; Löschung wird bei Verwendung mit Zähler verweigert.",
      },
      {
        key: "catalog.jeton-enforcement",
        title: "JETON-Modus erzwingt Zuweisung",
        action: "POS-Modus JETON aktivieren und ein aktives Produkt ohne Jeton aktivieren/anlegen.",
        expected:
          "Aktivierung wird blockiert («Jeton ist Pflicht»); /admin/settings warnt mit Anzahl fehlender Zuweisungen.",
      },
      {
        key: "catalog.menu-create",
        title: "Bundle mit Slots aufbauen",
        action:
          "Unter /admin/bundles ein Bundle mit 2 Slots (z. B. Hauptgang + Getränk) samt Optionen und Mengen anlegen und publizieren.",
        expected:
          "Slots/Optionen speichern korrekt; unpubliziertes Bundle bleibt im Shop unsichtbar, publiziertes erscheint.",
      },
      {
        key: "catalog.menu-shop",
        title: "Bundle-Konfiguration im Shop",
        action: "Das Bundle im Storefront öffnen und konfigurieren.",
        expected:
          "Modal zeigt Slots in Reihenfolge mit Optionen inkl. Preis/Lagerstatus; Konfiguration landet korrekt im Warenkorb.",
      },
    ],
  },
  {
    key: "tenant-settings",
    title: "10 · Tenant-Einstellungen (/admin/settings)",
    haveReady: ["app_admin- oder Owner-Login"],
    tests: [
      {
        key: "tenant-settings.system-toggle",
        title: "System ein-/ausschalten",
        action: "«Bestellsystem» deaktivieren, Shop aufrufen, wieder aktivieren.",
        expected:
          "Deaktiviert zeigt der Shop «Aktuell geschlossen» und Checkout ist unmöglich; aktiviert funktioniert alles.",
      },
      {
        key: "tenant-settings.pos-mode",
        title: "POS-Modus wechseln",
        action: "Zwischen QR_CODE, JETON und HYBRID wechseln.",
        expected:
          "Wechsel persistiert; JETON verlangt vollständige Jeton-Zuweisungen (sonst Warnung); Auswirkung auf Belegdruck folgt in Abschnitt 15.",
      },
    ],
  },
  {
    key: "shop",
    title: "11 · Kunden-Shop & TWINT-Checkout",
    haveReady: [
      "Publizierte Produkte inkl. Depot-Produkt und Bundle",
      "Payrexx vollständig konfiguriert (Abschnitt 5)",
      "Smartphone mit TWINT",
      "Testpostfach für die Quittung",
    ],
    tests: [
      {
        key: "shop.browse",
        title: "Storefront-Anzeige",
        action: "/ auf dem Tenant-Host öffnen und das Grid prüfen.",
        expected:
          "Produkte nach Kategorieposition sortiert; Preis/Depot korrekt; Badges «Ausverkauft», «Nur X übrig» (≤10) und «Nicht aktiv» erscheinen situationsgerecht.",
      },
      {
        key: "shop.cart",
        title: "Warenkorb-Bedienung",
        action: "Produkte hinzufügen, Mengen ändern, Position entfernen, Bundle-Position umkonfigurieren.",
        expected:
          "Zähler und Summen aktualisieren live; leerer Korb blendet die Bottom-Nav aus; Bundle-Änderung persistiert.",
      },
      {
        key: "shop.totals",
        title: "Summen mit Depot",
        action: "Depot-Produkt (Menge 2) in den Korb legen und Checkout-Zusammenfassung prüfen.",
        expected: "Zwischensumme, Depot-Zeile und Gesamtsumme sind getrennt ausgewiesen und rechnerisch korrekt.",
      },
      {
        key: "shop.checkout-empty",
        title: "Leerer Checkout",
        action: "/checkout mit leerem Korb öffnen.",
        expected: "«Warenkorb ist leer»-Zustand; Bezahl-Button deaktiviert.",
      },
      {
        key: "shop.guest-payment",
        title: "Gast-Checkout startet TWINT",
        action: "Ohne Login zur Kasse, optionale E-Mail angeben, «Jetzt mit TWINT zahlen».",
        expected: "Bestellung + Payment werden erstellt; Redirect auf die Payrexx-Checkout-Seite.",
      },
      {
        key: "shop.twint-success",
        title: "TWINT-Zahlung erfolgreich",
        action: "Zahlung in TWINT bestätigen und zurückleiten lassen.",
        expected:
          "Statusseite pollt bis «bezahlt», Korb wird geleert, Erfolgsseite erscheint; Bestellseite zeigt den Abhol-QR-Code.",
      },
      {
        key: "shop.twint-cancel",
        title: "TWINT-Zahlung abgebrochen",
        action: "Auf der Payrexx-Seite abbrechen.",
        expected: "Abbruchseite mit «Erneut versuchen»; Warenkorb bleibt erhalten; Bestellung bleibt unbezahlt.",
      },
      {
        key: "shop.payment-error",
        title: "Zahlungsfehler",
        action: "Fehlerfall provozieren (z. B. Gateway-Timeout/Offline nach Redirect).",
        expected: "Fehlerseite mit Retry; Korb bleibt erhalten; kein Geldabfluss ohne bezahlte Bestellung.",
      },
      {
        key: "shop.receipt-email",
        title: "Quittungs-E-Mail",
        action: "Nach erfolgreicher Zahlung das Postfach prüfen.",
        expected:
          "Betreff «Deine Quittung von <Marke>»; Positionen inkl. Bundle-Konfiguration und Depot; Summen korrekt; QR-Link nutzt die Standard-Domain des Tenants.",
      },
      {
        key: "shop.orders-list",
        title: "Bestellliste (Gast + eingeloggt)",
        action: "/orders als Gast (gleicher Browser) und als eingeloggter Kunde öffnen.",
        expected:
          "Gast sieht lokal gespeicherte Bestellungen, eingeloggt kommen sie vom Backend; Detailseite je erreichbar.",
      },
      {
        key: "shop.order-detail",
        title: "Bestelldetail mit Live-Status",
        action: "Bestelldetail offen lassen, während Erstattung/Abholung (spätere Abschnitte) passieren.",
        expected:
          "Positionen, Depot, Zahlungsdaten (Methode, ggf. Karte + Tx-Code) korrekt; 10-Sekunden-Polling zeigt Statusänderungen ohne Reload.",
      },
      {
        key: "shop.logged-in-checkout",
        title: "Checkout mit Login",
        action: "Eingeloggt bestellen.",
        expected: "E-Mail vorausgefüllt; Bestellung erscheint in der Backend-Historie des Kontos.",
      },
    ],
  },
  {
    key: "pos-pairing",
    title: "12 · POS-Gerät & Stationen koppeln",
    haveReady: [
      "Android-Tablet mit aktueller POS-APK (richtiger Kanal)",
      "Setup-PIN des Builds",
      "Laptop mit /admin für die Freigabe",
    ],
    tests: [
      {
        key: "pos-pairing.pin-gate",
        title: "PIN-Gate des Geräte-Setups",
        action: "Auf dem POS-Homescreen die obere linke Ecke lange drücken; falsche, dann richtige PIN eingeben.",
        expected: "Falsche PIN: «Falsche PIN.», Feld leert sich. Richtige PIN öffnet das Setup.",
      },
      {
        key: "pos-pairing.host-validation",
        title: "Tenant-Host-Validierung",
        action:
          "Ungültige Hosts versuchen: comanda.leys.ch (Apex), www.…, http://…, localhost; danach den gültigen Tenant-Host speichern.",
        expected:
          "Ungültige Eingaben zeigen deutsche Fehlermeldungen und speichern nicht; gültiger Host lädt https://<host>/pos.",
      },
      {
        key: "pos-pairing.host-qr",
        title: "Host per QR-Code setzen",
        action: "«QR-Code scannen» nutzen (Kamera-Berechtigung erteilen) und einen Host-QR scannen.",
        expected: "Gescannter Wert füllt das Host-Feld; Validierung wie bei manueller Eingabe.",
      },
      {
        key: "pos-pairing.approve",
        title: "Gerät koppeln und freigeben",
        action: "POS lädt und zeigt den Pairing-Code; unter /admin/pos das pendente Gerät freigeben.",
        expected:
          "Gerät wechselt auf «Angenommen» und die Kasse wird funktionsfähig; Liste sortiert pending zuerst, Token maskiert.",
      },
      {
        key: "pos-pairing.host-tools",
        title: "Zuletzt verwendete Hosts & Reset",
        action: "Host wechseln, «Zuletzt verwendet» nutzen, «Auf Standard zurücksetzen» klicken.",
        expected:
          "Bis zu 5 letzte Hosts wählbar; Tenant-Wechsel wischt WebView-Session/Cookies und bietet SumUp-Logout an; Reset lädt die Build-URL.",
      },
      {
        key: "pos-pairing.revoke",
        title: "Gerät entziehen",
        action: "Unter /admin/pos das Gerät entfernen und am Tablet weiterarbeiten wollen.",
        expected: "Gerät verliert den Zugriff (Re-Pairing nötig); Status «Entzogen» in der Liste.",
      },
      {
        key: "pos-pairing.stations",
        title: "Station koppeln und bestücken",
        action:
          "Unter /admin/stations ein Stationsgerät koppeln, umbenennen, Produkte zuweisen/entfernen, danach entziehen.",
        expected: "Alle Aktionen persistieren; entzogene Station kann keine Abholungen mehr verarbeiten.",
      },
    ],
  },
  {
    key: "pos-sumup",
    title: "13 · Android-POS: SumUp-Kartenzahlung",
    haveReady: [
      "SumUp-Reader (Solo/Air) geladen, Bluetooth aktiv",
      "Korrektes SumUp-Konto + idealerweise ein zweites (falsches) Konto",
      "Merchant-Code im Hub hinterlegt (Abschnitt 6)",
      "Echte Karte für eine Testzahlung",
    ],
    tests: [
      {
        key: "pos-sumup.login",
        title: "SumUp-Anmeldung auf dem Gerät",
        action: "Im Geräte-Setup «SumUp-Konto anmelden» und mit dem korrekten Konto einloggen.",
        expected: "Login gelingt (Affiliate-Key im Build); Setup zeigt das aktive Konto an.",
      },
      {
        key: "pos-sumup.guardrail-match",
        title: "Guardrail: Konto stimmt",
        action: "Mit hinterlegtem Merchant-Code und korrektem Konto den Status im Setup prüfen.",
        expected: "Grüner/neutraler Status «Angemeldet als <code>»; Kartenzahlung freigegeben.",
      },
      {
        key: "pos-sumup.guardrail-mismatch",
        title: "Guardrail: falsches Konto blockiert",
        action: "Mit dem falschen SumUp-Konto einloggen und im Web-POS eine Kartenzahlung starten.",
        expected:
          "Setup zeigt rot «Falsches Konto: angemeldet als …, erwartet …»; der Checkout wird mit Fehlermeldung blockiert, der Reader öffnet sich nie.",
      },
      {
        key: "pos-sumup.guardrail-unset",
        title: "Guardrail: ohne Code nur Hinweis",
        action: "Merchant-Code im Hub leeren (Abschnitt 6), POS neu laden, Kartenzahlung starten.",
        expected: "Setup zeigt nur einen Hinweis («Kein erwartetes SumUp-Konto hinterlegt»); Zahlung ist möglich.",
      },
      {
        key: "pos-sumup.payment",
        title: "Kartenzahlung Happy-Path",
        action: "POS-Bestellung mit «Karte» abschliessen; Karte am Reader tappen.",
        expected:
          "SumUp-Checkout öffnet mit korrektem Betrag/Titel; Erfolg meldet ans Web-POS zurück; Bestellung ist bezahlt.",
      },
      {
        key: "pos-sumup.payment-details",
        title: "Transaktionsdaten in der Bestellung",
        action: "Die Kartenbestellung unter /admin/orders öffnen.",
        expected:
          "Kartenmarke, letzte 4 Ziffern, Entry-Mode und SumUp-Transaktionscode sind gespeichert und auch per Suche auffindbar.",
      },
      {
        key: "pos-sumup.payment-abort",
        title: "Abbruch/Fehlschlag am Reader",
        action: "Eine Kartenzahlung am Reader abbrechen.",
        expected: "Web-POS zeigt den Fehler; Bestellung bleibt unbezahlt; erneuter Versuch möglich.",
      },
      {
        key: "pos-sumup.dashboard-logout",
        title: "SumUp-Dashboard & Logout",
        action: "Transaktion im SumUp-Dashboard prüfen; danach im Setup «Abmelden».",
        expected:
          "Transaktion erscheint mit korrektem Titel/Referenz beim richtigen Merchant; nach Logout zeigt das Setup «Nicht bei SumUp angemeldet» und Kartenzahlung verlangt Login.",
      },
    ],
  },
  {
    key: "pos-printing",
    title: "14 · Android-POS: Belegdruck",
    haveReady: [
      "Thermodrucker (ESC/POS Bluetooth und/oder Phomemo T02), in Android gekoppelt",
      "Tenant-Logo hochgeladen (Abschnitt 4)",
    ],
    tests: [
      {
        key: "pos-printing.discovery",
        title: "Drucker suchen und koppeln",
        action:
          "Im POS die Druckersuche starten (Bluetooth-Berechtigungen erteilen), den Drucker im Pairing-Modus koppeln.",
        expected:
          "Gefundene Geräte erscheinen mit Name/Adresse; Bonding klappt und der Drucker taucht in der Liste gebondeter Drucker auf.",
      },
      {
        key: "pos-printing.selection",
        title: "Druckerauswahl persistiert",
        action: "Drucker auswählen, App neu starten, erneut drucken.",
        expected: "Auswahl bleibt gespeichert (printer_mac); Druck geht ohne Neuauswahl an denselben Drucker.",
      },
      {
        key: "pos-printing.receipt",
        title: "Verkaufsbeleg drucken",
        action: "Nach einer POS-Zahlung den Beleg drucken.",
        expected:
          "Beleg enthält Logo/Name/Adresse des Tenants, Positionen inkl. Depot, Summen, Zahlungsmethode und — je nach POS-Modus — den Abhol-QR.",
      },
      {
        key: "pos-printing.phomemo",
        title: "Phomemo-Rasterdruck",
        action: "Denselben Beleg auf dem Phomemo T02 drucken (falls vorhanden).",
        expected: "Raster-Pfad wird erkannt (Gerätename), Beleg ist vollständig und lesbar.",
      },
      {
        key: "pos-printing.refund-slip",
        title: "Erstattungs-/Storno-Beleg",
        action: "Nach einer Erstattung (Abschnitt 16) den Beleg drucken.",
        expected: "Slip zeigt Erstattungsart, Positionen/Betrag und Referenz zur Bestellung.",
      },
      {
        key: "pos-printing.printer-off",
        title: "Drucker ausgeschaltet",
        action: "Druck auslösen, während der Drucker aus ist.",
        expected: "Klarer Fehler im POS; kein Hänger; Druck nach Einschalten wiederholbar.",
      },
      {
        key: "pos-printing.logo-fallback",
        title: "Logo nicht erreichbar",
        action: "Logo-URL unerreichbar machen (z. B. Offline-Test) und drucken.",
        expected: "Beleg druckt ohne Logo weiter; kein Absturz.",
      },
    ],
  },
  {
    key: "pos-fulfillment",
    title: "15 · POS-Bestellungen & Abholung",
    haveReady: ["Gekoppeltes POS + Station (Abschnitt 12)", "Bei JETON-Modus: Jetons physisch bereit"],
    tests: [
      {
        key: "pos-fulfillment.cash",
        title: "Barzahlung an der Kasse",
        action: "POS-Bestellung mit «Bar» abschliessen, gegebenen Betrag erfassen.",
        expected:
          "Rückgeld wird korrekt berechnet; Beleg druckt; Bestellung erscheint mit Ursprung POS und Methode CASH.",
      },
      {
        key: "pos-fulfillment.card",
        title: "Kartenzahlung an der Kasse",
        action: "POS-Bestellung mit Karte (Abschnitt 13) abschliessen.",
        expected: "Ablauf wie Barzahlung, Methode CARD inkl. Tx-Daten.",
      },
      {
        key: "pos-fulfillment.pickup",
        title: "Abholung per QR an der Station",
        action:
          "Den Abhol-QR einer bezahlten Shop-Bestellung an der Station scannen; parallel die Kunden-Bestellseite offen halten.",
        expected:
          "Redemption wird erfasst; Kundenseite zeigt «Abgeholt» innert ~10 s (Polling); erneuter Scan wird als bereits eingelöst abgewiesen.",
      },
      {
        key: "pos-fulfillment.qr-suppressed",
        title: "Kein QR für stornierte/erstattete Bestellungen",
        action: "Bestelldetail einer stornierten und einer voll erstatteten Bestellung öffnen.",
        expected: "Statt QR erscheint der Hinweis, dass kein Abhol-Code existiert; Status-Badge stimmt.",
      },
      {
        key: "pos-fulfillment.jeton-mode",
        title: "JETON-Modus ohne QR",
        action: "Im JETON-Modus eine POS-Bestellung abschliessen.",
        expected: "Beleg druckt ohne Abhol-QR; Ausgabe läuft über die zugewiesenen Jetons.",
      },
    ],
  },
  {
    key: "orders-refunds",
    title: "16 · Bestellverwaltung & Erstattungen",
    haveReady: [
      "Je eine bezahlte TWINT-, Bar- und Kartenbestellung (Abschnitte 11/15)",
      "Eine Bestellung mit Depot-Positionen",
    ],
    tests: [
      {
        key: "orders-refunds.search",
        title: "Bestellliste und Suche",
        action: "/admin/orders öffnen; nach Bestell-ID, Kunden-E-Mail und Karten-Transaktionscode suchen.",
        expected:
          "Liste zeigt Status/Summe/Ursprung/Methode; jede Suchart filtert korrekt (Debounce, keine Fremdtreffer).",
      },
      {
        key: "orders-refunds.detail",
        title: "Bestelldetail",
        action: "Eine Bundle-Bestellung öffnen.",
        expected:
          "Positionsbaum zeigt Eltern-Artikel mit Komponenten; Zahlungen mit Methode und Details; Summen konsistent.",
      },
      {
        key: "orders-refunds.pending-transitions",
        title: "Pendente Bestellung überführen",
        action: "Eine pendente Bestellung als bezahlt markieren; eine weitere stornieren.",
        expected:
          "Statuswechsel greifen sofort; stornierte Bestellung gibt reservierten Bestand frei (Ledger cancellation).",
      },
      {
        key: "orders-refunds.twint-refund",
        title: "TWINT-Positions-Erstattung",
        action: "Bei der TWINT-Bestellung «Positionen erstatten», Menge + Grund wählen, auslösen; Webhook abwarten.",
        expected:
          "Refund startet pending und wechselt per Webhook auf succeeded; Kundenseite zeigt «Erstattet»-Badge und neue Summen.",
      },
      {
        key: "orders-refunds.cash-refund",
        title: "Bar-Erstattung",
        action: "Bei der Barbestellung eine Position erstatten.",
        expected: "Refund steht sofort auf succeeded (kein Async); Beträge stimmen.",
      },
      {
        key: "orders-refunds.card-refund",
        title: "Karten-Erstattung als Auszahlung",
        action: "Bei der SumUp-Kartenbestellung eine Erstattung auslösen.",
        expected: "Ablauf als Barauszahlung erfasst (SDK kennt keinen Refund-Call); Verbuchung und Beleg korrekt.",
      },
      {
        key: "orders-refunds.arbitrary",
        title: "Betrags-Erstattung",
        action: "«Betrag erstatten» mit einem Teilbetrag + Grund nutzen.",
        expected: "Exakter Betrag wird erstattet; Bestellung wechselt auf partially_refunded; Restsumme stimmt.",
      },
      {
        key: "orders-refunds.deposit-return",
        title: "Depot-Rückgabe",
        action: "«Depot zurückgeben» für die Depot-Positionen auslösen.",
        expected:
          "Depot-Betrag wird erstattet; Inventar-Ledger bucht +Menge mit Grund refund; Kundenseite zeigt «Depot zurückerstattet».",
      },
      {
        key: "orders-refunds.refunds-list",
        title: "Erstattungsliste",
        action: "/admin/refunds öffnen.",
        expected:
          "Alle Refunds mit Status-Badges (pending/succeeded/failed), Art (Position/Depot/Betrag/Storno), Methode und Auslöser; Link zurück zur Bestellung.",
      },
      {
        key: "orders-refunds.inventory-ledger",
        title: "Ledger-Gründe vollständig",
        action: "/admin/inventory für ein durchgetestetes Produkt öffnen.",
        expected:
          "Einträge opening_balance, sale, refund, manual_adjust (und ggf. cancellation) mit Delta, Verursacher und Bestell-Link.",
      },
      {
        key: "orders-refunds.dashboard",
        title: "Dashboard spiegelt den Testtag",
        action: "/admin öffnen und den Testtag auswerten.",
        expected:
          "Umsatz, Topprodukte, Zahlungsmix (CASH/CARD/TWINT), Shop/POS-Split und Stornos entsprechen den durchgeführten Tests.",
      },
    ],
  },
  {
    key: "staff-perks",
    title: "17 · Mitarbeiter-Gutscheine (Kampagnen)",
    haveReady: [
      "Produkte für die Kampagne",
      "Stationsgerät für die Einlösung",
      "Zweites Gerät/Inkognito für die Claim-Seite",
      "Optional: Drucker für Voucher-PDF",
    ],
    tests: [
      {
        key: "staff-perks.create",
        title: "Kampagne anlegen und publizieren",
        action:
          "Unter /admin/staff-perks eine Kampagne mit Produkten (inkl. Mengen), Max-Einlösungen und Ablauf anlegen; publizieren.",
        expected: "Kampagne startet als Entwurf und wird aktiv; Detailseite zeigt Zugangscode, Claim-Link und Zähler.",
      },
      {
        key: "staff-perks.claim-gate",
        title: "Claim-Seite mit Zugangscode",
        action: "Claim-Link im Inkognito öffnen; falschen, dann richtigen 4er-Code eingeben.",
        expected:
          "Falscher Code: Shake + «Code ist falsch.»; Zeichensatz ohne 0/O/I/l; richtiger Code lädt die Kampagne.",
      },
      {
        key: "staff-perks.claim-qr",
        title: "Claim-QR mit Produktliste",
        action: "Nach dem Code die Ansicht prüfen.",
        expected: "Signierter QR (Ed25519) wird angezeigt; Produktliste mit Mengen stimmt.",
      },
      {
        key: "staff-perks.redeem",
        title: "Einlösung an der Station",
        action: "QR an der Station scannen; Kampagnen-Detail parallel offen halten; bis zum Maximum wiederholen.",
        expected:
          "Zähler steigt live (5-s-Poll); Bestand der Produkte wird abgebucht; über dem Maximum wird abgewiesen.",
      },
      {
        key: "staff-perks.rotate",
        title: "Claim-Link erneuern",
        action: "«Link erneuern» bestätigen und den alten Link testen.",
        expected: "Alter Link ist ungültig; Einlösezähler bleibt erhalten; neuer Link funktioniert.",
      },
      {
        key: "staff-perks.vouchers",
        title: "Voucher-PDF drucken",
        action: "Über das Drucker-Icon z. B. 5 Voucher generieren.",
        expected: "PDF mit QR-Codes + Zugangscode und Anleitung öffnet sich; Codes sind einlösbar.",
      },
      {
        key: "staff-perks.end",
        title: "Kampagne beenden",
        action: "«Beenden» bestätigen und eine weitere Einlösung versuchen.",
        expected: "Status «beendet»; keine Einlösungen mehr möglich.",
      },
    ],
  },
  {
    key: "isolation",
    title: "18 · Tenant-Isolation & Sicherheit",
    haveReady: ["Zweiter minimaler Tenant mit eigenen Daten", "curl oder Proxy-Tool für Host-Header-Tests"],
    tests: [
      {
        key: "isolation.data",
        title: "Keine Daten über Tenant-Grenzen",
        action:
          "Auf Host von Tenant B Katalog/Bestellungen prüfen; zusätzlich per curl mit manipuliertem Host-Header von A nach B greifen.",
        expected: "Nur Tenant-B-Daten sichtbar; der Header-Trick liefert keine fremden Zeilen (leer/403) — RLS greift.",
      },
      {
        key: "isolation.membership",
        title: "Fremder Admin hat keinen Zugriff",
        action: "Mit dem Admin-Konto von Tenant A /admin auf dem Host von Tenant B öffnen.",
        expected: "Zugriff verweigert (keine Mitgliedschaft in Org B).",
      },
      {
        key: "isolation.webhook",
        title: "Webhook-Zuordnung ist tenant-fest",
        action:
          "Payrexx-Webhook mit Token von Tenant A gegen eine Bestellung von Tenant B senden; zusätzlich einen Request mit falscher Signatur.",
        expected: "Falscher Token: 404/kein Effekt. Falsche Signatur: 401; keine Bestellung ändert sich.",
      },
      {
        key: "isolation.platform-endpoints",
        title: "Plattform-Endpunkte für Tenant-Admins gesperrt",
        action: "Als Tenant-Owner GET /api/v1/platform/tenants aufrufen.",
        expected: "403 — is_platform_admin bleibt die einzige Eintrittskarte.",
      },
      {
        key: "isolation.suspended",
        title: "Gesperrter Tenant überall dicht",
        action: "Tenant B sperren und Shop, /admin, POS-Endpunkte und Systemstatus testen.",
        expected: "Alle Oberflächen blockieren konsistent; nach Reaktivierung wieder frei.",
      },
    ],
  },
  {
    key: "ops",
    title: "19 · Plattform-Betrieb & E-Mails",
    haveReady: ["Postfach-Zugriff", "Sentry-Dashboard", "GitHub-Release mit android-<kanal>-v*-Tag"],
    tests: [
      {
        key: "ops.emails",
        title: "Alle E-Mail-Typen rendern korrekt",
        action: "OTP-Login, Rollen-Einladung, Inhaber-Einladung und Quittung je einmal auslösen und die Mails prüfen.",
        expected:
          "Deutsch, Europe-Zurich-Zeiten, [Staging]-Präfix, Tenant-Branding; Kundenlinks auf der Tenant-Standard-Domain, Invite-Links auf dem Plattform-Apex bzw. Tenant-Host gemäss Konzept.",
      },
      {
        key: "ops.android-latest",
        title: "Android-Update-Feed",
        action: "GET /v1/android/latest?channel=staging (und production) aufrufen.",
        expected:
          "Antwort mit versionName/versionCode/downloadUrl/sha256 des neuesten passenden Release-Tags; zweiter Aufruf kommt aus dem 5-Minuten-Cache.",
      },
      {
        key: "ops.app-update",
        title: "In-App-Update",
        action: "Ältere APK installieren, App starten, Update-Dialog bestätigen.",
        expected:
          "Download läuft, SHA-256 wird geprüft, System-Installer öffnet; nach Installation läuft die neue Version.",
      },
      {
        key: "ops.sentry",
        title: "Sentry erhält Fehler mit Kontext",
        action: "Einen kontrollierten Fehler auslösen (z. B. Webhook mit falscher Signatur) und Sentry prüfen.",
        expected: "Event erscheint mit Endpoint/Tenant-Kontext; keine Secrets oder PII im Payload.",
      },
      {
        key: "ops.qr-keys",
        title: "QR-Schlüssel pro Tenant",
        action: "GET /v1/qr-config für zwei Tenants vergleichen.",
        expected:
          "Beide liefern einen Ed25519-Public-Key; die Schlüssel unterscheiden sich pro Tenant und bleiben über Aufrufe stabil.",
      },
    ],
  },
]

export const QA_TEST_COUNT = QA_SECTIONS.reduce((n, s) => n + s.tests.length, 0)

export function qaTestKeys(): string[] {
  return QA_SECTIONS.flatMap((s) => s.tests.map((t) => t.key))
}
