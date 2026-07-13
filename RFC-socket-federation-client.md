# RFC — Client du socket de fédération (côté instance)

Statut : draft, à implémenter après relecture. Contrepartie côté hub déjà en
place : `internal/ws` + `GET /api/v1/instances/me/stream` dans le repo
`immerle-hub` (voir son `docs/RFC-socket-federation.md` pour le protocole
complet). Ce document ne redéfinit pas le protocole, il décrit comment le
brancher dans le code existant de `internal/federation`.

## 1. Rappel de l'existant (ce qu'on modifie)

Aujourd'hui, deux mécanismes REST indépendants, tous les deux pilotés par
`federation.Service.Run` (tick horaire, `internal/federation/client.go:325`) :

- **Push** : `PlaylistSyncer` (`internal/federation/sync.go`), consommateur
  `outbox` (`internal/outbox/worker.go`) déclenché à chaque mutation de
  playlist publique. Résout les covers (`resolveCovers`) puis
  `PUT /api/v1/instances/me/playlists/{externalId}` via `hub.Client.SyncPlaylist`.
- **Pull** : `Service.SyncPlaylists` → `syncFeed` (`client.go:391`), pagine
  `GET .../feed/playlists` + `GET .../{id}/playlists/{externalId}` une fois par
  heure, matérialise en playlists locales `Federated: true` via
  `upsertFederated`.

## 2. Ce qui change

- Le **push** passe du `PUT` REST à une frame `playlist.upsert`/`playlist.delete`
  envoyée sur le socket. Les covers restent en REST (`MissingCovers`/
  `UploadCover` inchangés — décision du RFC hub §9).
- Le **pull horaire** est remplacé par la **réception live** des mêmes frames
  relayées par le hub depuis les instances suivies, plus un `resume` envoyé à
  la connexion pour rattraper ce qui a été manqué. `SyncPlaylists`/`syncFeed`
  ne disparaissent pas : ils deviennent le **fallback** utilisé quand le socket
  est indisponible (ancien hub, réseau qui bloque les upgrades WS, etc.) — voir
  §7.
- Le tick horaire de `Service.Run` garde son rôle pour le heartbeat REST de
  secours et le fallback, mais le heartbeat "normal" devient les frames
  `heartbeat`/`heartbeat_ack` du socket.

## 3. Nouveau paquet `internal/federation/stream`

```
internal/federation/stream/
  client.go     // connexion, reconnexion+backoff, dispatch
  protocol.go   // les mêmes types Frame que internal/ws/protocol.go côté hub
```

Le protocole n'étant pas dans l'OpenAPI vendorisé (`internal/federation/hub/openapi.json`
ne décrit pas les frames WS — l'upgrade lui-même y apparaît sans schéma utile),
`protocol.go` est écrit à la main, copie fidèle de `Frame` côté hub :

```go
package stream

type Frame struct {
    Type string `json:"type"`

    Cursors map[string]string `json:"cursors,omitempty"`

    ExternalID string          `json:"externalId,omitempty"`
    Version    string          `json:"version,omitempty"`
    UpdatedAt  string          `json:"updatedAt,omitempty"`
    Image      string          `json:"image,omitempty"`
    Tracks     json.RawMessage `json:"tracks,omitempty"`
    Metadata   json.RawMessage `json:"metadata,omitempty"`
    AuthorID   string          `json:"authorId,omitempty"`
    Target     string          `json:"target,omitempty"`

    ForSubscriberID string `json:"forSubscriberId,omitempty"`
    SinceVersion    string `json:"sinceVersion,omitempty"`

    Code    string `json:"code,omitempty"`
    Message string `json:"message,omitempty"`
}
```

`Client` (dial + reconnexion) :

```go
type Client struct {
    auth       func() hub.Auth        // réutilise federation.Service.auth()
    hubURL     func() string          // réutilise federation.Service.cfg().HubURL
    onUpsert   func(ctx context.Context, authorID string, f Frame) error
    onDelete   func(ctx context.Context, authorID string, f Frame) error
    onReplay   func(ctx context.Context, forSubscriberID, sinceVersion string) error
    cursors    CursorStore            // §4
    logger     *slog.Logger
}

func (c *Client) Run(ctx context.Context) {
    for {
        if err := c.connectAndServe(ctx); err != nil {
            c.logger.Warn("federation stream disconnected", "error", err)
        }
        if ctx.Err() != nil {
            return
        }
        time.Sleep(backoffWithJitter(...)) // cf. edge case thundering herd
    }
}
```

`connectAndServe` : `websocket.Dial` vers
`wss(s)://<hubURL>/api/v1/instances/me/stream` avec les mêmes en-têtes que le
client REST (`X-Instance-ID`, `Authorization: Bearer <privateKey>`, via
`websocket.DialOptions.HTTPHeader`), puis boucle : `SetReadLimit` 1 MiB (même
limite que le hub), heartbeat toutes les 25s dans une goroutine dédiée, lecture
bloquante + dispatch par `Frame.Type` (mêmes cases que côté hub : `welcome` →
envoie `resume` ; `playlist.upsert`/`delete` → `onUpsert`/`onDelete` ;
`replay.request` → `onReplay` ; `heartbeat_ack` → rien ; `error` → log).

`New(...)` est appelé depuis `federation.New` (`client.go:94`) et son `Run`
démarré à côté de `Service.Run` dans `internal/app` (même emplacement que
l'actuel `go federationService.Run(ctx)`), **seulement si** `s.HubConfigured()`
au démarrage — sinon il boucle en attente comme le fait déjà `Service.Run`
pour le tick (le lien peut se faire après coup depuis l'admin ; il faut alors
démarrer le client une fois `Register`/bootstrap réussi, pas seulement au
process start).

## 4. Curseur de rattrapage (côté abonné, pas côté hub — cf. RFC hub §3/§8)

Nouvelle table (migration `00037_federation_feed_cursor.sql`), même style que
`playlist_sync` :

```sql
CREATE TABLE federation_feed_cursor (
    source_instance_id TEXT PRIMARY KEY,
    last_version       TEXT NOT NULL,
    updated_at          INTEGER NOT NULL
);
```

Nouveau repo `FeedCursorRepo` (`internal/persistence/federation_feed_cursor.go`),
même forme que `PlaylistSyncRepo` : `Get(ctx, sourceInstanceID) (string, error)`,
`Set(ctx, sourceInstanceID, version string) error`. Au `welcome`, le client
construit `resume.cursors` à partir de `Service.Subscriptions(ctx)` (déjà
existant, `client.go:231`) croisé avec `FeedCursorRepo` — une instance suivie
sans entrée envoie une version vide (catch-up complet, cf. RFC hub §8a).

`Version` = `RFC3339Nano` de l'`UpdatedAt` de la playlist source côté publisher
(comparaison lexicographique = comparaison chronologique, pas besoin d'un
compteur dédié). Sur réception d'un `playlist.upsert`, comparer à la version
déjà connue pour cette playlist avant d'appliquer (protège du réordonnancement,
RFC hub edge case 12) puis `FeedCursorRepo.Set(sourceInstanceID, version)`.

## 5. Réception : brancher sur la matérialisation existante

`onUpsert`/`onDelete` réutilisent **exactement** `materializeFeed`/suppression
déjà écrites pour le pull REST (`client.go:442` `materializeFeed`,
`upsertFederated` à `client.go:465`) — aucune nouvelle logique de
matérialisation, seulement une nouvelle entrée dans le flux qui les appelle.
`upsertFederated` est déjà idempotent sur `(sourceInstanceID, sourceExternalID)`
donc une frame reçue deux fois (retry, doublon de reconnexion) ne pose pas de
problème.

## 6. Réponse à un `replay.request` (nous en tant que publisher)

Reçu quand un de nos abonnés vient de se reconnecter et demande le
rattrapage. On n'a **pas besoin de recalculer** covers/mosaïque : `PlaylistSyncRepo`
connaît déjà, pour chaque playlist actuellement synced, le dernier payload
poussé au hub (voir §8 — actuellement seul le hash est stocké, il faut aussi
garder le dernier payload résolu, cf. changement de schéma ci-dessous) :

```go
func (c *Client) handleReplayRequest(ctx context.Context, forSubscriberID, sinceVersion string) {
    ids, _ := c.syncState.IDs(ctx)
    for _, id := range ids {
        payload, version, _ := c.syncState.LastPayload(ctx, id) // nouveau, cf. §8
        if version <= sinceVersion {
            continue // l'abonné a déjà cet état ou plus récent
        }
        c.send(Frame{Type: TypePlaylistUpsert, ExternalID: id, Version: version, Target: forSubscriberID, ...})
    }
}
```

## 7. `PlaylistSyncer` : push via le socket, covers inchangées

`syncPlaylist` (`sync.go:146`) garde `resolveCovers` tel quel (REST), mais
remplace l'appel final `s.fed.hub.SyncPlaylist(...)` par un envoi de frame sur
le socket (`stream.Client.SendUpsert(...)`). Si le socket n'est pas connecté au
moment de l'envoi : retourner une erreur mappée par `handle` vers
`outbox.ErrNotReady` (même traitement que "hub pas encore lié",
`sync.go:105`) — le job outbox est retenté, exactement le mécanisme déjà en
place, sans rien inventer.

`deletePlaylist` (`sync.go:134`) : idem, `playlist.delete` sur le socket au
lieu de `hub.DeletePlaylist`.

## 8. Changement de schéma pour `PlaylistSyncRepo`

Actuellement `playlist_sync` ne stocke que `last_payload_hash` (pour sauter un
sync inchangé). Pour répondre à un `replay.request` sans tout recalculer, il
faut aussi garder le **payload résolu** (covers déjà en URLs hub) et sa
version :

```sql
ALTER TABLE playlist_sync ADD COLUMN last_payload TEXT NOT NULL DEFAULT '{}';
ALTER TABLE playlist_sync ADD COLUMN last_version TEXT NOT NULL DEFAULT '';
```

`Set` prend le payload JSON + version en plus du hash ; `LastPayload(ctx, id)`
nouveau getter. Pas de nouvelle table : c'est une extension naturelle de
l'existant, pas un nouveau concept.

## 9. Fallback REST (compat hub pas encore mis à jour / réseau restrictif)

Le hub RFC (§10) garde les endpoints REST en place pendant la migration —
côté instance, le principe symétrique : si le socket ne s'établit pas après N
tentatives (ex: 5) ou reste down plus de `federationSyncInterval` (1h,
`client.go:319`), retomber sur l'ancien chemin **déjà présent et non
supprimé** : `Service.Run` continue d'appeler `syncOnce`/`SyncPlaylists` sur
son tick horaire dans tous les cas, socket connecté ou non. Ce n'est donc pas
un mode dégradé à coder à part : c'est simplement ne rien retirer du chemin
REST existant, le socket vient en plus (best-effort, prioritaire quand
disponible). Le seul changement dans `PlaylistSyncer` est le point §7 (le push
d'un item outbox échoue en `ErrNotReady` si le socket est down au lieu de
pousser en REST) — accepté comme tradeoff : le pull REST horaire rattrape de
toute façon les autres instances, et le push REST reste un filet de sécurité
simple à réactiver (revert de §7 seul) si le socket s'avère peu fiable en
pratique.

## 10. Edge cases spécifiques au client

1. **Reconnexion en boucle serrée** si le hub redémarre pendant que beaucoup
   d'instances sont connectées (thundering herd, RFC hub edge 7, symétrique
   côté client) — `backoffWithJitter` exponentiel (même formule que
   `outbox.backoff`, `worker.go:153`) plutôt qu'un intervalle fixe.
2. **Hub qui ne supporte pas encore `/instances/me/stream`** (version hub plus
   ancienne) — `websocket.Dial` échoue à l'upgrade (404/426), traité comme une
   déconnexion normale : backoff et retry indéfiniment, le fallback REST
   (§9) couvre l'instance entre-temps.
3. **Instance qui se délie (`Unlink`, `client.go:292`) pendant que le socket
   est connecté** — fermer explicitement la connexion du `stream.Client` dans
   `Unlink`, sinon elle reste ouverte avec des identifiants révoqués côté hub
   jusqu'au prochain heartbeat raté (50s, tolérable mais autant fermer tout de
   suite).
4. **Réception d'une frame pour une playlist désabonnée entre-temps** — le hub
   est déjà censé filtrer ça via `instance_subscriptions` (RFC hub edge 8/9) ;
   côté client, `materializeFeed` ne doit pas re-créer une playlist fédérée
   pour un `authorId` qu'on ne suit plus localement (double-check avec
   `Service.Subscriptions` avant matérialisation, défense en profondeur bon
   marché).
5. **Rotation de clé privée (`RotateOwned` côté hub) pendant une connexion
   active** — l'ancienne clé devient invalide, le prochain `Dial` (au reconnect
   suivant, ou immédiatement si on ferme proactivement sur un changement de
   config détecté) échoue en 401 ; le fallback REST échouera aussi tant que la
   nouvelle clé n'est pas persistée localement (`saveCreds`) — non spécifique
   au socket, déjà vrai aujourd'hui pour le REST.

## 11. Plan de test

Même approche que côté hub (`internal/ws/hub_test.go`) : un test du
`stream.Client` contre un faux serveur WS (`httptest.NewServer` + un handler
minimal jouant le rôle du hub), vérifiant le dispatch (`onUpsert`/`onDelete`/
`onReplay` appelés avec les bons arguments) sans dépendre de la vraie
connexion réseau au hub. Un test séparé pour `FeedCursorRepo`/`PlaylistSyncRepo.LastPayload`
(SQLite en mémoire, même pattern que les autres repos).

## 12. Découpage suggéré (petits PRs, chacun testable seul)

1. Migration + `FeedCursorRepo` + extension `PlaylistSyncRepo` (§4, §8) — pas
   de comportement changé, juste le stockage.
2. `internal/federation/stream` (client, protocole, dispatch) branché en
   **lecture seule** : reçoit et matérialise (§3, §5), envoie `resume`/`heartbeat`,
   mais le push (§7) reste en REST pour l'instant.
3. Bascule du push vers le socket (§7) + réponse aux `replay.request` (§6).
4. Fermeture propre sur `Unlink` (§10.3) + garde-fou subscription (§10.4).
