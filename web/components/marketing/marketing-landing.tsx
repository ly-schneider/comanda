import { ArrowRight, ArrowUpRight, Blocks, Check } from "lucide-react"
import Image from "next/image"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { CONTACT_EMAIL, CONTACT_MAILTO, LINKS, SECTIONS } from "@/lib/marketing"
import { SiteFooter } from "./site-footer"
import { SiteNav } from "./site-nav"
import { BrandingVisual, OrderingVisual, PickupVisual, StockVisual, TillVisual } from "./visuals"

const container = "mx-auto w-full max-w-[1200px] px-4 sm:px-6 xl:px-0"
const h2 = "text-[34px] leading-[40px] font-semibold tracking-[-0.025em] text-balance sm:text-5xl sm:leading-[52px]"
const lead = "text-muted-foreground text-[17px] leading-[28px] sm:text-[19px] sm:leading-[30px]"
const surface = "rounded-[var(--radius-surface)]"

export function MarketingLanding() {
  return (
    <>
      <SiteNav />
      <main>
        <Hero />
        <InUse />
        <HowItWorks />
        <Story />
        <Contact />
      </main>
      <SiteFooter />
    </>
  )
}

function Hero() {
  return (
    <section className="pt-16 pb-24 sm:pt-24 sm:pb-32">
      <div className={`${container} flex flex-col items-center text-center`}>
        <h1 className="text-[44px] leading-[1.05] font-semibold tracking-[-0.035em] text-balance sm:text-6xl lg:text-[76px] lg:leading-[78px]">
          Comanda ist Kasse und
          <br className="hidden sm:block" /> Handy-Bestellung
          <br className="hidden sm:block" /> für deinen Stand.
        </h1>
        <p className={`${lead} mt-6 max-w-[560px] text-pretty`}>
          Gäste bestellen am Handy oder an der Kasse. Ich habe Comanda gebaut, und ich richte es für deinen Anlass ein.
        </p>
        <div className="mt-8 flex flex-wrap justify-center gap-3">
          <Button asChild variant="primary" size="lg">
            <a href={CONTACT_MAILTO}>
              Schreib mir
              <ArrowRight />
            </a>
          </Button>
          <Button asChild variant="secondary" size="lg" className="rounded-pill">
            <a href={LINKS.blogFirst}>So hat es angefangen</a>
          </Button>
        </div>
      </div>
      <div className={`${container} relative mt-16`}>
        <div className={`relative aspect-[4/3] overflow-hidden sm:aspect-[1200/620] ${surface}`}>
          <Image
            src="/assets/images/marketing/kasse-festival.jpg"
            alt="Comanda-Kasse am Festival: Tablet, SumUp-Kartenleser und Bon-Drucker"
            fill
            priority
            sizes="(min-width: 1200px) 1200px, 100vw"
            className="object-cover"
          />
        </div>
        <OrderCard />
      </div>
    </section>
  )
}

function OrderCard() {
  const lines = [
    ["1× Cappuccino", "4.50"],
    ["1× Croissant", "3.50"],
    ["1× Mehrwegbecher (Depot)", "2.00"],
  ]
  return (
    <div
      aria-hidden
      className={`bg-background relative mx-4 -mt-24 p-6 shadow-[0_12px_32px_rgba(0,0,0,0.12)] sm:absolute sm:right-10 sm:bottom-10 sm:m-0 sm:w-80 ${surface}`}
    >
      <p className="text-[15px] font-semibold">Bestellung 142</p>
      <p className="text-muted-foreground text-[13px]">Kaffeebar</p>
      <div className="mt-3.5">
        {lines.map(([item, price]) => (
          <div key={item} className="border-border flex justify-between border-t py-2.5 text-[13px]">
            <span>{item}</span>
            <span className="text-muted-foreground">{price}</span>
          </div>
        ))}
        <div className="border-border flex justify-between border-t pt-3 pb-4 text-[15px] font-semibold">
          <span>Total</span>
          <span>CHF 10.00</span>
        </div>
      </div>
      <div className="bg-success/10 text-success flex items-center justify-center gap-2 rounded-[var(--radius-control)] py-2.5 text-[13px] font-medium">
        <Check className="size-4" />
        Bezahlt mit TWINT
      </div>
    </div>
  )
}

const STATS = [
  ["4'051", "Bestellungen an fünf Festivaltagen"],
  ["95 %", "zahlten mit TWINT. Die Kartenleser waren fast Deko."],
  ["800", "Gäste pro Abend am Jugendanlass"],
  ["0", "Burger verkauft, die es nicht mehr gab"],
]

function InUse() {
  return (
    <section id={SECTIONS.inUse} className="scroll-mt-20 pb-24 sm:pb-32">
      <div className={`${container} grid gap-4 lg:grid-cols-2`}>
        <div
          className={`bg-secondary flex min-h-[360px] flex-col justify-between gap-10 p-8 sm:p-10 lg:min-h-[440px] ${surface}`}
        >
          <h2 className={h2}>Bisher lief Comanda an zwei Anlässen.</h2>
          <div>
            <p className="text-muted-foreground max-w-[440px] text-[15px] leading-6">
              Zuerst lief es an einem monatlichen Jugendanlass mit 800 Gästen. Im Juli 2026 lief es dann fünf Tage an
              einem Festival, auch am Merch-Stand.
            </p>
            <a
              href={LINKS.blogSecond}
              className="mt-4 inline-flex items-center gap-1.5 text-[13px] font-medium hover:underline"
            >
              Wie es am Festival lief
              <ArrowRight className="size-3.5" />
            </a>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {STATS.map(([value, label]) => (
            <div
              key={value}
              className={`bg-secondary flex min-h-[180px] flex-col justify-between gap-6 p-7 lg:min-h-[212px] ${surface}`}
            >
              <p className="text-[56px] leading-[60px] font-medium tracking-[-0.03em]">{value}</p>
              <p className="text-muted-foreground text-[15px] leading-6">{label}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

const FEATURES = [
  {
    visual: <OrderingVisual />,
    title: "Gäste bestellen am Handy",
    text: "Sie scannen den QR-Code am Stand, wählen aus und zahlen mit TWINT. Eine App brauchen sie dafür nicht.",
  },
  {
    visual: <PickupVisual />,
    title: "Gäste holen mit einem Abholcode ab",
    text: "Nach dem Bezahlen hat jeder Gast einen Abholcode auf dem Handy. Am Ausgabefenster scannt ihn dein Team mit dem eigenen Handy. Das klappt auch, wenn das Festival-WLAN gerade streikt.",
  },
  {
    visual: <TillVisual />,
    title: "Wer bar oder mit Karte zahlt, geht an die Kasse",
    text: "Die Kasse ist ein Tablet mit einem SumUp-Kartenleser und einem kleinen Bon-Drucker. Die Hardware für zwei Kassen kostete unter 1'000 Franken.",
  },
  {
    visual: <StockVisual />,
    title: "Du weisst, was noch da ist",
    text: "Ist der letzte Burger weg, können ihn die Gäste nicht mehr bestellen. Nach dem Anlass siehst du, was wann verkauft wurde. Damit planst du den Einkauf fürs nächste Mal.",
  },
  {
    visual: <BrandingVisual />,
    title: "Der Bestellshop trägt deinen Namen",
    text: "Deine Gäste sehen dein Logo und deine Farben unter deiner eigenen Adresse. Der Name Comanda erscheint im Shop nicht.",
  },
]

const EXTRAS = [
  [
    "Depot auf Becher",
    "Du legst ein Depot fest. Am Festival waren es zwei Franken pro Mehrwegbecher. Bei der Rückgabe geht das Geld über die TWINT-Zahlung zurück.",
  ],
  [
    "Gutscheine für Artists",
    "Artists bekommen gedruckte Gutscheine und tauschen sie am Stand gegen Essen und Getränke ein.",
  ],
  ["Essen fürs Team", "Dein Team bekommt sein Essen gratis, und es ist trotzdem verbucht."],
  ["Export für die Buchhaltung", "Nach dem Anlass bekommst du alle Verkäufe als Excel-Datei."],
]

function HowItWorks() {
  return (
    <section id={SECTIONS.howItWorks} className="scroll-mt-20 pb-24 sm:pb-32">
      <div className={container}>
        <h2 className={h2}>So funktioniert&apos;s.</h2>
        <p className={`${lead} mt-4 max-w-[640px]`}>
          Die meisten Funktionen gibt es, weil Leute hinter dem Stand danach gefragt haben.
        </p>
        <div className="mt-12">
          {FEATURES.map((feature) => (
            <div
              key={feature.title}
              className="border-border grid items-center gap-8 border-t py-10 last:border-b lg:grid-cols-2 lg:gap-20"
            >
              {feature.visual}
              <div>
                <h3 className="text-2xl leading-8 font-semibold tracking-[-0.02em] sm:text-[30px] sm:leading-9">
                  {feature.title}
                </h3>
                <p className={`${lead} mt-4 max-w-[440px]`}>{feature.text}</p>
              </div>
            </div>
          ))}
        </div>
        <div className="pt-12">
          <h3 className="text-[22px] leading-7 font-semibold tracking-[-0.015em]">Was sonst noch dazugehört</h3>
          <div className="mt-6 grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
            {EXTRAS.map(([title, text]) => (
              <div key={title} className="border-border border-t pt-5">
                <p className="text-[15px] font-semibold">{title}</p>
                <p className="text-muted-foreground mt-2 text-[15px] leading-6">{text}</p>
              </div>
            ))}
          </div>
        </div>
        <div className="pt-12">
          <h3 className="text-[22px] leading-7 font-semibold tracking-[-0.015em]">Was Comanda nicht kann</h3>
          <p className={`${lead} mt-4 max-w-[720px]`}>
            Comanda ist für Stände gebaut, die ein paar Stunden oder Tage verkaufen. Tische, offene Rechnungen und
            Reservationen gibt es nicht, und die Kasse läuft nur auf Android. Für ein Restaurant sind SumUp POS oder
            Lightspeed die bessere Wahl, und beide haben ein Support-Team, das ich nicht habe.
          </p>
        </div>
      </div>
    </section>
  )
}

const POSTS = [
  {
    title: "Building a checkout system at 16",
    text: "Angefangen hat es mit 1'000 Franken Budget.",
    href: LINKS.blogFirst,
  },
  {
    title: "The checkout system got a second life",
    text: "An fünf Festivaltagen ging auch einiges kaputt.",
    href: LINKS.blogSecond,
  },
]

function Story() {
  return (
    <section id={SECTIONS.story} className="scroll-mt-20 pb-24 sm:pb-32">
      <div className={`${container} grid items-center gap-12 lg:grid-cols-[500px_1fr] lg:gap-20`}>
        <div className={`bg-secondary flex aspect-[500/520] flex-col justify-between p-8 sm:p-10 ${surface}`}>
          <p className="text-muted-foreground max-w-[300px] text-[15px] leading-6">
            So alt war ich, als ich gefragt habe, ob ich eine neue Kasse bauen darf.
          </p>
          <p className="text-[160px] leading-[150px] font-semibold tracking-[-0.07em] sm:text-[240px] sm:leading-[220px]">
            16
          </p>
        </div>
        <div>
          <blockquote className="text-[26px] leading-[34px] font-semibold tracking-[-0.02em] sm:text-[34px] sm:leading-[44px]">
            «Samstag, 12:07. Sechzehn Bestellungen pro Minute, und ich stehe hinter dem Stand und schaue aufs Handy,
            statt in Panik zu geraten.»
          </blockquote>
          <div className="mt-8 text-[15px] leading-6">
            <p className="font-semibold">Levyn Schneider</p>
            <p className="text-muted-foreground">hat Comanda gebaut und betreut es</p>
          </div>
          <div className="mt-8">
            {POSTS.map((post) => (
              <a
                key={post.href}
                href={post.href}
                className="border-border group flex items-center justify-between gap-6 border-t py-4.5 last:border-b"
              >
                <span>
                  <span className="block text-[15px] font-semibold group-hover:underline">{post.title}</span>
                  <span className="text-muted-foreground block text-[13px]">{post.text}</span>
                </span>
                <ArrowUpRight className="size-[18px] shrink-0" />
              </a>
            ))}
            <p className="text-muted-foreground mt-3.5 text-[13px]">Beide Texte sind auf Englisch.</p>
          </div>
        </div>
      </div>
    </section>
  )
}

function Contact() {
  return (
    <section id={SECTIONS.contact} className="scroll-mt-20 pb-24 sm:pb-32">
      <div className={`${container} flex flex-col items-center`}>
        <h2 className={`${h2} text-center`}>Es gibt kein Abo.</h2>
        <p className={`${lead} mt-4 max-w-[600px] text-center`}>
          Comanda kannst du nicht online buchen, und monatlich zahlst du auch nichts. Schreib mir, wenn du es an deinem
          Anlass einsetzen willst.
        </p>
        <div className="mt-12 grid w-full gap-4 md:grid-cols-2">
          <ContactOption
            icon={<Image src="/assets/images/comanda-logo.png" alt="" width={24} height={24} />}
            title="Ich richte es für dich ein"
            text="Wir gehen dein Sortiment zusammen durch und verbinden dein Payrexx- und dein SumUp-Konto. Wenn du willst, stehe ich am Anlass mit am Stand."
            action={
              <Button asChild variant="primary" size="lg">
                <a href={CONTACT_MAILTO}>
                  Mail schreiben
                  <ArrowRight />
                </a>
              </Button>
            }
            note={CONTACT_EMAIL}
          />
          <ContactOption
            icon={<Blocks className="size-6" strokeWidth={1.75} />}
            title="Selber betreiben"
            text="Der Code ist öffentlich. Vereine und kleine Organisationen dürfen ihn gratis selber betreiben. Grössere Organisationen brauchen eine Lizenz."
            action={
              <Button asChild variant="outline" size="lg">
                <Link href="/lizenz">Lizenz lesen</Link>
              </Button>
            }
            note="Business Source License 1.1"
          />
        </div>
      </div>
    </section>
  )
}

function ContactOption({
  icon,
  title,
  text,
  action,
  note,
}: {
  icon: React.ReactNode
  title: string
  text: string
  action: React.ReactNode
  note: string
}) {
  return (
    <div className={`bg-secondary flex flex-col items-center gap-4 px-8 py-12 text-center sm:px-12 ${surface}`}>
      <span className="bg-background rounded-pill p-3.5">{icon}</span>
      <h3 className="text-[22px] leading-7 font-semibold tracking-[-0.015em]">{title}</h3>
      <p className="text-muted-foreground max-w-[400px] text-[15px] leading-6">{text}</p>
      <div className="mt-2">{action}</div>
      <p className="text-muted-foreground text-[13px]">{note}</p>
    </div>
  )
}
