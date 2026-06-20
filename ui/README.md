# Immerle — application Expo cross-platform

Une seule base de code Expo / React Native + TypeScript ciblant **web, iOS et
Android**. Client musical compatible Subsonic / OpenSubsonic, avec une couche
Immerle *capability-aware* qui masque les fonctions absentes selon ce que
l'instance annonce.

## Stack

- **Expo SDK 52** + React Native 0.76 + TypeScript (strict)
- **expo-router** (routing par fichiers, partagé web/natif)
- **Zustand** (état) + **TanStack Query** (data-fetching/cache)
- **NativeWind** (Tailwind pour RN) — thème clair/sombre via variables CSS
- **expo-secure-store** (tokens, fallback `localStorage` sur web)
- Couche audio abstraite : `react-native-track-player` (natif) /
  `HTMLAudioElement` + MediaSession (web) derrière une interface commune
- **FlashList** pour les listes virtualisées (bibliothèques 10k+ titres)

## Lancer

```bash
npm install
npm run web      # navigateur
npm run ios      # simulateur iOS  (macOS + Xcode)
npm run android  # émulateur Android
```

Connexion : URL de l'instance Subsonic/Immerle + identifiant + mot de passe.
La session est persistée (secure-store) et restaurée au redémarrage. Le mot de
passe ne quitte jamais l'écran de login : seul le **token salé** dérivé
(`md5(password + salt)`) est stocké.

### Builds natifs (EAS)

`eas.json` définit les profils `development` / `preview` / `production`.

```bash
npx eas build --profile development --platform ios      # build dev client
npx eas build --profile production --platform all
```

> iOS réel nécessite un compte développeur Apple. **Tester l'audio en
> arrière-plan sur device physique** : le simulateur ne reflète pas le
> comportement lockscreen / interruptions.

## Architecture

```
app/                      Routes expo-router (écrans)
  (tabs)/                 Accueil · Bibliothèque · Recherche · Playlists · Admin · Réglages
  album|artist|genre|playlist/[id]   Détails
  player.tsx · queue.tsx  Lecteur plein écran + file (modaux)
src/
  api/subsonic/           Client Subsonic typé (auth token salé, URLs stream/cover)
  api/immerle/          Client capability-aware + endpoints admin étendus
  audio/                  Abstraction AudioPlayer (engine.native / engine.web) + store
  auth/                   Store de session + stockage sécurisé cross-platform
  query/                  Hooks TanStack Query (library, search, playlists, admin)
  components/             UI réutilisable (TrackList, CoverArt, MiniPlayer, …)
  theme/                  Tokens couleur + préférence clair/sombre
```

### Couche capability-aware

`probeCapabilities()` interroge `/immerle/capabilities`. En cas d'échec (404,
réseau, serveur Subsonic pur) elle retombe sur un jeu **conservateur**
(`SUBSONIC_ONLY_CAPABILITIES`) : l'app se réduit gracieusement à un client
musical. Chaque fonction Immerle est gardée par `client.has(feature)`, donc
l'UI n'affiche jamais d'impasse.

### Lecteur

| Plateforme | Moteur | Apport OS |
|---|---|---|
| iOS / Android | `react-native-track-player` | lecture en arrière-plan, contrôles lockscreen/notification, now-playing |
| Web | `HTMLAudioElement` + MediaSession API | contrôles OS / touches média, métadonnées |

File réordonnable, play/pause/seek, répétition, choix de **qualité/transcodage**
(mappé sur `maxBitRate`/`format` Subsonic), **scrobbling** (now-playing +
soumission au-delà de 50 % ou 4 min), synchro **`savePlayQueue`** (debounced)
pour suivre l'écoute entre appareils.

## Statut des jalons

| Jalon | État |
|---|---|
| **W0** Fondations (scaffold, router, API typé, auth+secure-store, thème, onglets, EAS) | ✅ |
| **W1** Bibliothèque (artistes/albums/pistes/genres, détails, recherche live, cache pochettes, pull-to-refresh, listes virtualisées) | ✅ |
| **W2** Lecteur cross-platform (abstraction 2 moteurs, file, qualité, scrobbling, savePlayQueue) | ✅ |
| **W3** Playlists (CRUD, ajout/retrait partout, réordonnancement, détail) | ✅ |
| **W4** Administration (users CRUD+reset MDP, scan+progression+stats, providers+jobs, fédération, serveur/transcodage — gaté par rôle) | ✅ |
| **W5** Social + catalogue à la demande (Jam, flux, playlists collaboratives, recherche distante) | ⏳ à venir |
| **W6** Reco/éditoriales, offline, finition (PWA, cache hors-ligne) | 🚧 offline natif + PWA faits ; reco/éditoriales à venir |

> Les écrans admin providers/jobs/fédération/serveur dépendent d'endpoints
> Immerle côté serveur (S4/S5/S7). Ils sont câblés et capability-gated : ils
> apparaissent uniquement quand l'instance les annonce.

## Validation

```bash
npm run typecheck   # tsc --noEmit  → OK
npm test            # jest (md5, formatters, capabilities) → OK
npx expo export --platform web   # bundle web statique → OK (27 routes)
```

> Conformité OpenSubsonic : tester contre des **clients réels** à chaque jalon
> API, pas seulement en unitaire.

## Légalité & confidentialité

- **Catalogue à la demande** : les téléchargements proviennent de **providers
  tiers configurés par l'utilisateur**. Leur usage relève de sa responsabilité
  et de la législation applicable (cf. avertissement dans l'écran Providers).
- **Fédération** : tout ce qui remonte au hub est **anonymisé et opt-in**
  (désactivé par défaut). Traité comme une exigence, pas une option.

## Notes d'implémentation

- **NativeWind est épinglé à `4.1.23`** (css-interop `0.1.x`, plugin
  `react-native-reanimated/plugin`). La série 4.2.x (css-interop 0.2.x) exige
  `react-native-worklets/plugin` (Reanimated 4), incompatible avec Reanimated
  3.16 livré par SDK 52.
