import { COMPANY } from "@openagents/shared/constants";
import type { Metadata } from "next";

const LAST_UPDATED = "19 August 2026";

const description =
  "How Open Agents handles data in the desktop app, CLI, and orchestrator.inc: local-first operation, waitlists, and testimonial submissions.";

export const metadata: Metadata = {
  title: "Privacy Policy",
  description,
  openGraph: {
    type: "article",
    url: `${COMPANY.MARKETING_URL}/privacy/`,
    siteName: COMPANY.NAME,
    title: `Privacy Policy | ${COMPANY.NAME}`,
    description,
    images: [
      {
        url: `${COMPANY.MARKETING_URL}/og-image.png`,
        width: 1200,
        height: 630,
        alt: `${COMPANY.NAME} privacy policy`,
      },
    ],
  },
  twitter: {
    card: "summary",
    title: `Privacy Policy | ${COMPANY.NAME}`,
    description,
    images: [`${COMPANY.MARKETING_URL}/og-image.png`],
  },
  alternates: {
    canonical: `${COMPANY.MARKETING_URL}/privacy/`,
  },
};

const ISSUES_URL = COMPANY.REPORT_ISSUE_URL;
const DISCORD_URL = COMPANY.DISCORD_URL;

function Section({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section
      id={id}
      className="scroll-mt-28 border-t border-border pt-10 first:border-t-0 first:pt-0"
    >
      <h2 className="text-[22px] font-semibold leading-tight tracking-[-0.02em] text-foreground sm:text-[26px]">
        {title}
      </h2>
      <div className="mt-5 space-y-4 text-[15px] leading-[1.75] text-muted-foreground sm:text-[16px]">
        {children}
      </div>
    </section>
  );
}

function Bullets({ children }: { children: React.ReactNode }) {
  return <ul className="space-y-3 pl-0">{children}</ul>;
}

function Bullet({ children }: { children: React.ReactNode }) {
  return (
    <li className="relative pl-5 before:absolute before:left-0 before:top-[0.7em] before:h-1 before:w-1 before:rounded-full before:bg-primary">
      {children}
    </li>
  );
}

function Strong({ children }: { children: React.ReactNode }) {
  return <strong className="font-semibold text-foreground">{children}</strong>;
}

function Ext({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="text-foreground underline decoration-primary decoration-1 underline-offset-4 transition-colors hover:text-primary"
    >
      {children}
    </a>
  );
}

function Code({ children }: { children: React.ReactNode }) {
  return (
    <code className="rounded-[4px] border border-border bg-muted/30 px-1.5 py-0.5 font-mono text-[0.86em] text-foreground">
      {children}
    </code>
  );
}

const toc = [
  { id: "scope", label: "What this covers" },
  { id: "desktop", label: "Desktop app & CLI" },
  { id: "website", label: "This website" },
  { id: "not-collected", label: "Data we do not collect" },
  { id: "third-parties", label: "Third-party services" },
  { id: "security", label: "Storage & security" },
  { id: "retention", label: "Retention & deletion" },
  { id: "rights", label: "Your rights" },
  { id: "children", label: "Children" },
  { id: "changes", label: "Changes" },
  { id: "contact", label: "Contact" },
];

export default function PrivacyPage() {
  return (
    <main className="relative min-h-screen">
      <header className="relative">
        <div className="relative mx-auto max-w-3xl px-6 pb-10 pt-16 md:pb-12 md:pt-20">
          <div className="font-mono text-sm tracking-[0.5px] text-muted-foreground">
            Legal
          </div>
          <h1 className="mt-4 text-[clamp(34px,5vw,54px)] font-semibold leading-[1.04] tracking-[-0.03em] text-foreground">
            Privacy Policy
          </h1>
          <p className="mt-4 font-mono text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
            Last updated {LAST_UPDATED}
          </p>

          <div className="mt-8 rounded-[8px] border border-border bg-card/50 p-6 sm:p-7">
            <p className="text-[15px] leading-[1.75] text-muted-foreground sm:text-[16px]">
              <Strong>The short version.</Strong> Open Agents runs on your
              own machine. No account is required, and no hosted Open Agents service stores
              your work. We never see your source code, prompts, agent output,
              terminal contents, repository names, or file paths, and we never
              sell or rent data to anyone. The desktop app and this website send{" "}
              <Strong>no telemetry and run no analytics</Strong>. If you
              voluntarily join a waitlist or send us a testimonial, we process
              the details you submit only for the purpose described on that
              form. The product runs locally and sends no product telemetry.
            </p>
          </div>

        </div>
      </header>

      <div className="relative mx-auto max-w-3xl px-6 pb-24 pt-12">
        <nav aria-label="On this page">
          <h2 className="font-mono text-[10px] uppercase tracking-[0.22em] text-muted-foreground">
            On this page
          </h2>
          <ul className="mt-4 grid gap-x-8 gap-y-2 sm:grid-cols-2">
            {toc.map((item) => (
              <li key={item.id}>
                <a
                  href={`#${item.id}`}
                  className="text-[14px] text-muted-foreground transition-colors hover:text-foreground"
                >
                  {item.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>

        <div className="mt-14 space-y-10">
          <Section id="scope" title="What this policy covers">
            <p>
              Open Agents is open-source software published by the
              Open Agents project. This policy applies to:
            </p>
            <Bullets>
              <Bullet>
                <Strong>The Open Agents desktop app and CLI</Strong> — the local
                manager that supervises coding agents in git worktrees on
                your computer.
              </Bullet>
              <Bullet>
                <Strong>orchestrator.inc</Strong> — this website and the
                documentation hosted on it.
              </Bullet>
            </Bullets>
            <p>
              Open Agents is not a hosted service. There is no Open Agents account system and no
              Open Agents server that stores your work. Everything Open Agents orchestrates —
              repositories, worktrees, sessions, terminals, agent output — lives
              on hardware you control.
            </p>
            <p>
              The AI coding agent you run inside Open Agents (OpenCode) is a separate
              third-party tool with its own privacy policy. Open Agents launches it
              locally; it does not intercept, store, or forward what it sends to
              its own provider.
            </p>
          </Section>

          <Section id="desktop" title="Desktop app and CLI">
            <p>
              The desktop app and CLI run entirely on your machine. All
              application state — projects, worktrees, sessions, terminal
              history, settings — is written under <Code>~/.open-agents</Code> on your
              own disk and is never uploaded to us.
            </p>
            <p>
              The desktop app, CLI, and local daemon send{" "}
              <Strong>no telemetry of any kind</Strong>. Nothing leaves your
              machine for product analytics: no usage events, no crash reports,
              no version or OS probes. If you connect a GitHub account for
              pull-request and CI awareness, Open Agents uses your existing local GitHub
              credentials to talk to GitHub directly from your machine. Those
              credentials stay on your machine and are never transmitted to us.
            </p>
          </Section>

          <Section id="website" title="This website">
            <p>
              orchestrator.inc is a static site and runs{" "}
              <Strong>no advertising and no analytics</Strong>. It sets no
              tracking cookies and loads no third-party analytics scripts, so
              nothing about your visit is collected or stored beyond the
              standard server logs described below.
            </p>
            <p>
              Voluntary waitlists and testimonial submissions are separate from
              any site behavior. When you submit one, the details requested by
              that form are sent to the relevant submission endpoint and stored
              solely to manage that request. Testimonial submissions include the
              testimonial, your public LinkedIn profile URL, and any optional
              public X post URL. We use those details to review and, with the
              permission granted on the form, publish your testimonial with
              public attribution on the Open Agents website. Fonts are self-hosted. Other
              services involved when you browse are:
            </p>
            <Bullets>
              <Bullet>
                <Strong>GitHub.</Strong> Your browser requests the public
                repository's star count and latest release from the GitHub API,
                which means GitHub sees the request.
              </Bullet>
              <Bullet>
                <Strong>Mux.</Strong> The product demo is played through an
                embedded Mux video player, which loads only when the page
                containing it is viewed.
              </Bullet>
            </Bullets>
            <p>
              Our hosting provider may keep standard server logs (IP address,
              user agent, requested URL) for security and abuse prevention, as
              any web server does. We do not use those logs to build profiles.
            </p>
          </Section>

          <Section id="not-collected" title="Data we do not collect">
            <Bullets>
              <Bullet>
                Your source code, diffs, commits, or repository contents.
              </Bullet>
              <Bullet>
                Your prompts, agent conversations, or agent output.
              </Bullet>
              <Bullet>
                Terminal contents, command history, or environment variables.
              </Bullet>
              <Bullet>
                File paths, project names, branch names, or repository names.
              </Bullet>
              <Bullet>
                API keys, tokens, passwords, or any other credential.
              </Bullet>
              <Bullet>
                Names or account information. The only email address, company
                role, or social profile we collect is information you
                voluntarily submit through an optional waitlist.
              </Bullet>
              <Bullet>Precise location data.</Bullet>
              <Bullet>
                Anything used for advertising, ad targeting, or cross-app
                tracking.
              </Bullet>
            </Bullets>
            <p>
              We do not sell, rent, or share personal data with third parties
              for their own purposes.
            </p>
          </Section>

          <Section id="third-parties" title="Third-party services">
            <p>
              Open Agents relies on a small number of services, each only to make a
              specific feature work:
            </p>
            <Bullets>
              <Bullet>
                <Strong>GitHub</Strong> — hosts the source code, releases, and
                this website (
                <Ext href="https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement">
                  privacy statement
                </Ext>
                ).
              </Bullet>
              <Bullet>
                <Strong>Mux</Strong> — serves the demo video on this site (
                <Ext href="https://www.mux.com/privacy">privacy policy</Ext>).
              </Bullet>
            </Bullets>
          </Section>

          <Section id="security" title="Storage and security">
            <p>
              All Open Agents state is stored under <Code>~/.open-agents</Code> on
              your own machine, protected by your operating system's file
              permissions. Because the server is one <Strong>you</Strong> run,
              you are responsible for securing that machine.
            </p>
            <p>
              No system is perfectly secure, but because Open Agents holds no central
              store of your data, there is no Open Agents-side database of user content
              that could be breached.
            </p>
          </Section>

          <Section id="retention" title="Data retention and deletion">
            <Bullets>
              <Bullet>
                <Strong>On your devices.</Strong> Data stays until you delete
                it. Deleting <Code>~/.open-agents</Code> removes all desktop
                state.
              </Bullet>
              <Bullet>
                <Strong>Waitlist details.</Strong> Retained only while needed to
                notify you about the relevant release or Open Agents Cloud access, then
                deleted. You may request earlier deletion using the private
                contact address below.
              </Bullet>
              <Bullet>
                <Strong>Testimonial submissions.</Strong> Retained while they
                are reviewed or displayed on the Open Agents website, including the
                supplied public LinkedIn and optional X post URLs. You may
                request deletion using the private contact address below.
              </Bullet>
            </Bullets>
          </Section>

          <Section id="rights" title="Your rights">
            <p>
              Depending on where you live, you may have rights to access,
              correct, export, or delete personal data about you, and to object
              to certain processing — for example under the GDPR or the
              CCPA/CPRA.
            </p>
            <p>
              In practice, nearly all data Open Agents touches is already in your own
              hands: delete <Code>~/.open-agents</Code>, and it is gone.
              If you submitted a waitlist email or believe we hold other data about
              you, contact us privately at{" "}
              <Ext href={COMPANY.MAIL_TO}>{COMPANY.MAIL_TO.replace("mailto:", "")}</Ext>{" "}
              and we will act on the request. We do not sell or share personal
              information as those terms are defined under US state privacy laws.
            </p>
          </Section>

          <Section id="children" title="Children">
            <p>
              Open Agents is a developer tool intended for professional and hobbyist
              software developers. It is not directed to children under 13, and
              we do not knowingly collect personal information from children.
            </p>
          </Section>

          <Section id="changes" title="Changes to this policy">
            <p>
              We may update this policy as Open Agents evolves. Material changes will be
              reflected here with a new "last updated" date, and the history of
              every revision is public in the project's git repository.
            </p>
          </Section>

          <Section id="contact" title="Contact">
            <p>
              Send privacy requests or information you do not want to make
              public to{" "}
              <Ext href={COMPANY.MAIL_TO}>{COMPANY.MAIL_TO.replace("mailto:", "")}</Ext>.
              General questions and corrections can also use these public
              channels:
            </p>
            <Bullets>
              <Bullet>
                <Ext href={ISSUES_URL}>Open a GitHub issue</Ext> — the fastest
                route, and the one we monitor.
              </Bullet>
              <Bullet>
                <Ext href={DISCORD_URL}>Join the Discord</Ext> — for questions
                that are not a bug report.
              </Bullet>
            </Bullets>
            <p className="text-muted-foreground">
              Open Agents is open-source software released under Apache
              2.0 and provided as-is. If this policy and the source code ever
              disagree, the source code is the truth — and you are welcome to
              read it.
            </p>
          </Section>
        </div>
      </div>
    </main>
  );
}
