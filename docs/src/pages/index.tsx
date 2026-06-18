import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import useBaseUrl from '@docusaurus/useBaseUrl';
import styles from './index.module.css';

const DOCKER_CMD = `docker run -d --name immerle \\
  -p 4533:4533 \\
  -v ./music:/music:ro \\
  -v immerle-data:/data \\
  ghcr.io/immerle/immerle:latest`;

const FEATURES = [
  {
    glyph: '🎧',
    title: 'Works with your clients',
    body: 'Full Subsonic / OpenSubsonic — browse, search, stream, transcode, playlists, scrobbling, now-playing. Supersonic, Symfonium and DSub just work.',
  },
  {
    glyph: '🌍',
    title: 'On-demand catalog',
    body: "Pluggable providers (Jamendo, Internet Archive, your own HTTP source) stream tracks you don't own yet, progressively on first play.",
  },
  {
    glyph: '👯',
    title: 'Social',
    body: 'Friends, an activity feed with per-event privacy, and collaborative or public playlists.',
  },
  {
    glyph: '🔊',
    title: 'Jam sessions',
    body: 'Listen together, in sync, streamed live.',
  },
  {
    glyph: '📥',
    title: 'Playlist import',
    body: 'Bring your playlists over — Spotify ships first.',
  },
  {
    glyph: '🔗',
    title: 'Federation, opt-in',
    body: 'Sync editorial & recommendation playlists across servers via an immerle-hub.',
  },
];

function Equalizer() {
  // Decorative; hidden from assistive tech.
  return (
    <span className={styles.eq} aria-hidden="true">
      {Array.from({length: 5}).map((_, i) => (
        <span key={i} style={{['--i' as string]: i}} />
      ))}
    </span>
  );
}

export default function Home(): React.ReactElement {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout title="Self-hosted music that sings" description={siteConfig.tagline}>
      <main className={styles.main}>
        <div className={styles.aurora} aria-hidden="true" />
        <div className={styles.grain} aria-hidden="true" />

        <section className={styles.hero}>
          <img
            className={styles.logo}
            src={useBaseUrl('/img/logo.svg')}
            alt="Immerle"
            width={104}
            height={85}
          />
          <p className={styles.kicker}>One small Go binary · up in minutes</p>
          <h1 className={styles.title}>
            Own every
            <br />
            beat of your <span className={styles.sings}>music</span>
            <Equalizer />
          </h1>
          <p className={styles.lede}>
            Stream, share and discover — on a music server that's entirely
            yours. No subscriptions, no limits, no lock-in.
          </p>

          <div className={styles.ctas}>
            <Link className={styles.primary} to="/docs">
              Read the docs
            </Link>
            <Link className={styles.secondary} href="https://github.com/immerle/immerle">
              View on GitHub ↗
            </Link>
          </div>

          <div className={styles.terminal}>
            <div className={styles.termBar} aria-hidden="true">
              <span /><span /><span />
              <em>drop in your music, hit play</em>
            </div>
            <pre className={styles.termBody}>
              <code>{DOCKER_CMD}</code>
            </pre>
          </div>
        </section>

        <section className={styles.features}>
          {FEATURES.map((f) => (
            <article key={f.title} className={styles.card}>
              <span className={styles.cardGlyph} aria-hidden="true">{f.glyph}</span>
              <h3 className={styles.cardTitle}>{f.title}</h3>
              <p className={styles.cardBody}>{f.body}</p>
            </article>
          ))}
        </section>

        <section className={styles.outro}>
          <h2 className={styles.outroTitle}>Up and running in a couple of minutes.</h2>
          <Link className={styles.primary} to="/docs/installation">
            Get started →
          </Link>
        </section>
      </main>
    </Layout>
  );
}
