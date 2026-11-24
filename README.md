# La Vallée Perdue

Jeu de ferme et d'aventure en pixel art, jouable dans le navigateur, avec un serveur autoritaire.
Le concept complet (univers, systèmes, roadmap) est dans le document de concept du projet.

Ce dépôt contient les **phases 0 et 1 du MVP** : une vallée en tuiles de 16 px où l'on se
déplace case par case, coupe, mine et fauche avec ses outils, laboure, sème, arrose et récolte,
au rythme d'un cycle jour/nuit, et où dormir sauvegarde la partie dans PostgreSQL.
Le serveur valide chaque action.

## Lancer le jeu en local

Il faut Go 1.24+, Node 22+ et, pour garder la vallée entre deux lancements, PostgreSQL 14+.

```sh
# 0. Une base vide (les tables sont créées au démarrage)
createdb lostvalley

# 1. Le serveur de jeu (port 8080)
cd server
go run ./cmd/lostvalley -db postgres://localhost/lostvalley
# ou LV_DATABASE_URL=postgres://… go run ./cmd/lostvalley

# 2. Le client, dans un autre terminal (port 5173, /ws est relayé vers le serveur)
cd client
npm install
npm run dev
```

Ouvre http://localhost:5173. Deux onglets du même navigateur pilotent **le même personnage**
(critère de sortie de la phase 0). Pour un deuxième joueur, ouvre une fenêtre privée ou ajoute
`?joueur=un-autre-nom` à l'adresse.

Sans `-db`, le serveur tourne en mémoire et prévient que rien ne sera gardé.

Commandes : flèches, ZQSD ou WASD pour se déplacer ; 1 à 9, la molette ou un clic pour choisir
un outil ou des graines ; Espace ou E pour l'utiliser sur la case en face. Face à une culture mûre,
E récolte quel que soit l'objet en main ; face au lit, E va dormir.

## Ce qu'on peut faire (phase 1)

- **Outils** (gardés à la mort) : houe pour labourer l'herbe, arrosoir (20 charges, se remplit
  face à la rivière), faux pour les herbes hautes, la friche et les cultures fanées, hache pour
  les arbres, pioche pour les rochers. Chaque coup coûte de l'énergie.
- **Cultures** : navet, blé et fève. Une culture arrosée gagne un jour de croissance chaque matin ;
  la pluie (20 % des jours) arrose tout.
- **Le temps passe hors ligne** : une journée dure 18 minutes en jeu, et 1 heure réelle quand la
  vallée est vide. Au retour, le serveur rattrape les jours écoulés d'un coup.
- **Abandon** : 3 jours sans eau et une culture fane ; une terre labourée laissée vide 4 jours
  part en friche ; une culture fanée ou mûre laissée en terre lâche ses graines, et certaines
  **mutent** en variante sauvage rare (navet-lune, blé d'ambre, fève-ronce) qui pousse sans eau.
- **Jour et nuit** : la nuit tombe de 18 h à 21 h. Si tout le monde dans la vallée est au lit, on
  passe au matin. Encore debout à 2 h, on s'effondre et on se réveille à la maison avec la moitié
  de son énergie.
- **Sauvegarde** : chaque matin, quand le dernier joueur part, toutes les minutes si quelque chose
  a changé, et à l'arrêt du serveur.

Tous ces réglages sont dans `assets/data/contenu.json`, lu par le serveur (règles) et le client
(noms) : on équilibre sans toucher au code. Le serveur le lit au démarrage (`-content`).

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
| `server/internal/world` | Règles pures et testées : carte, outils, cultures, horloge, sauvegarde en instantané |
| `server/internal/store` | Sauvegarde : PostgreSQL (vallée en JSONB, joueurs à part) ou mémoire |
| `server/internal/game` | Boucle d'une instance de domaine, diffusion des événements, sauvegardes |
| `server/internal/transport` | Connexions WebSocket, poignée de main `hello` |
| `server/internal/protocol` | Messages JSON (miroir TypeScript dans `client/src/net/protocol.ts`) |
| `server/cmd/genassets` | Génère l'art provisoire et la première carte |
| `client/` | Client Phaser 3 + TypeScript (Vite) ; HUD en HTML/CSS par-dessus le canvas |
| `assets/` | Cartes Tiled, images et données de jeu, lues **à la fois** par Phaser et par Go |

## Principes

- **Le serveur décide.** Le client envoie des intentions (`move`, `use`) et attend l'accusé
  `ack` avant l'action suivante. Il n'affiche que ce que le serveur confirme.
- **Le temps passe hors ligne.** L'horloge de la vallée avance même sans joueur (plus lentement),
  et les ressources repoussent selon l'heure réelle (arbre 3 min, rocher 5 min, herbes 90 s).
- **Une carte, deux lecteurs.** La carte est un export JSON de [Tiled](https://www.mapeditor.org/).
  Calques attendus : `sol` (tuiles, propriétés booléennes `bloquant`, `cultivable` et `eau`),
  `ressources` (objets de type `arbre`, `rocher` ou `herbes`) et `reperes` (objets `apparition`
  et `lit` ; la case sous le lit doit être libre).
- **Art original.** Tout l'art actuel est provisoire et dessiné par `genassets`. Une fois la carte
  retouchée dans Tiled, ne relance plus `genassets`, qui l'écraserait.

## Tests

```sh
cd server && go test -race ./...
cd client && npm test && npm run build
```

Le test PostgreSQL ne tourne que si `LV_TEST_DATABASE_URL` pointe vers une base jetable
(la CI en fournit une).

## Pas encore là

Saisons qui comptent pour les cultures (le calendrier défile, mais tout pousse en toute saison),
qualité des récoltes, vente et coffre de la maison, combat au pas et mort (phase 2), comptes et
visites entre domaines (phase 4), personnalisation du personnage.
