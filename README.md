# La Vallée Perdue

Jeu de ferme et d'aventure en pixel art, jouable dans le navigateur, avec un serveur autoritaire.
Le concept complet (univers, systèmes, roadmap) est dans le document de concept du projet.

Ce dépôt contient le **socle du MVP** (phase 0 de la roadmap, plus une première récolte) :
une vallée en tuiles de 16 px où l'on se déplace case par case et où l'on coupe des arbres,
casse des rochers et fauche les herbes hautes. Le serveur valide chaque action.

## Lancer le jeu en local

Il faut Go 1.24+ et Node 22+.

```sh
# 1. Le serveur de jeu (port 8080)
cd server
go run ./cmd/lostvalley

# 2. Le client, dans un autre terminal (port 5173, /ws est relayé vers le serveur)
cd client
npm install
npm run dev
```

Ouvre http://localhost:5173. Deux onglets du même navigateur pilotent **le même personnage**
(critère de sortie de la phase 0). Pour un deuxième joueur, ouvre une fenêtre privée ou ajoute
`?joueur=un-autre-nom` à l'adresse.

Commandes : flèches, ZQSD ou WASD pour se déplacer ; Espace ou E pour récolter la case en face.

Pour servir le client compilé directement depuis le serveur Go :

```sh
cd client && npm run build
cd ../server && go run ./cmd/lostvalley -static ../client/dist
# puis http://localhost:8080
```

## Organisation

| Dossier | Contenu |
| --- | --- |
| `server/` | Serveur Go : règles du monde, une goroutine par domaine, WebSocket (`coder/websocket`) |
| `server/internal/world` | Règles pures et testées : carte, déplacement, récolte, repousse |
| `server/internal/game` | Boucle d'une instance de domaine et diffusion des événements |
| `server/internal/transport` | Connexions WebSocket, poignée de main `hello` |
| `server/internal/protocol` | Messages JSON (miroir TypeScript dans `client/src/net/protocol.ts`) |
| `server/cmd/genassets` | Génère l'art provisoire et la première carte |
| `client/` | Client Phaser 3 + TypeScript (Vite) ; HUD en HTML/CSS par-dessus le canvas |
| `assets/` | Cartes Tiled et images, lues **à la fois** par Phaser (rendu) et par Go (logique) |

## Principes

- **Le serveur décide.** Le client envoie des intentions (`move`, `gather`) et attend l'accusé
  `ack` avant l'action suivante. Il n'affiche que ce que le serveur confirme.
- **Le temps passe hors ligne.** Les ressources repoussent selon l'heure réelle, même sans joueur
  connecté (arbre 3 min, rocher 5 min, herbes 90 s pour l'instant).
- **Une carte, deux lecteurs.** La carte est un export JSON de [Tiled](https://www.mapeditor.org/).
  Calques attendus : `sol` (tuiles, propriété booléenne `bloquant` sur les tuiles infranchissables),
  `ressources` (objets de type `arbre`, `rocher` ou `herbes`) et `reperes` (objet `apparition`).
- **Art original.** Tout l'art actuel est provisoire et dessiné par `genassets`. Une fois la carte
  retouchée dans Tiled, ne relance plus `genassets`, qui l'écraserait.

## Tests

```sh
cd server && go test -race ./...
cd client && npm test && npm run build
```

## Pas encore là

Comptes et sauvegarde PostgreSQL (phase 1 : pour l'instant tout est en mémoire et se perd au
redémarrage du serveur), outils et énergie, cultures, cycle jour/nuit, combat au pas, personnalisation
du personnage, visites entre domaines.
