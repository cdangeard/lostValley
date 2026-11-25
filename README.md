# La Vallée Perdue

Jeu de ferme et d'aventure en pixel art, jouable dans le navigateur, avec un serveur autoritaire.
Le concept complet (univers, systèmes, roadmap) est dans le document de concept du projet.

Ce dépôt contient les **phases 0 à 3 du MVP** : une vallée en tuiles de 16 px où l'on se
déplace case par case, coupe, mine et fauche avec ses outils, laboure, sème, arrose et récolte,
au rythme d'un cycle jour/nuit, et où dormir sauvegarde la partie dans PostgreSQL. Depuis la
phase 2, on y construit (atelier, puits, séchoir, épouvantail, clôtures), on agrandit le domaine
en dessouchant, et on se bat **au pas** contre les créatures de la lisière et la vague nocturne
qui vise les champs. Depuis la phase 3, chaque activité donne de l'expérience, chaque niveau un point
à placer dans la **Constellation des Racines**, et l'on apprivoise trois créatures de la forêt.
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
un outil, des graines ou de la nourriture ; Espace ou E pour l'utiliser sur la case en face. Face à
une culture mûre, E récolte quel que soit l'objet en main ; face au lit, E va dormir ; face au
coffre, E l'ouvre. B ouvre le menu de construction (1 à 9 pour choisir, E pour poser, Échap pour
fermer). Au pas, X passe son tour. T ouvre la Constellation des Racines. Face à une créature, E avec
sa nourriture en main la nourrit ; face à une de tes créatures, E avec autre chose lui dit de te suivre
ou de rester à la maison.

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

## Ce qu'on peut faire (phase 2)

- **Construire** (B) : l'**atelier** (2×2) débloque les autres plans : **puits** (remplit
  l'arrosoir), **séchoir** (2×1 : 6 récoltes deviennent des rations en une nuit), **épouvantail**
  (les corbeaux restent à 3 cases) et **clôtures** (arrêtent les bêtes qui marchent ; les
  rongeurs passent dessous, les corbeaux dessus, et une bête bloquée finit par les casser).
  Un bâtiment se pose sur la case en face, en terrain libre et **dans le domaine** ; les
  matériaux sortent du sac, et « Démonter » les rend.
- **Agrandir le domaine** : il commence en 12 × 12 cases autour de la maison (bordure dorée
  en mode construction). Une souche arrachée à la pioche ne repousse plus, et si elle touche
  le domaine, il s'étend de 2 cases autour d'elle.
- **Combat au pas** : dès qu'une créature est à 5 cases, le monde n'avance que quand tu joues.
  Un pas coûte 1 tick, la houe 1, la faux et la pioche 2, la hache 3 ; chaque créature agit
  selon sa vitesse. Avant de frapper, elle marque en rouge les cases visées : tu as un tour
  pour t'écarter ou la frapper. À deux joueurs au pas, celui qui tarde plus de 10 s passe son tour.
- **Quatre créatures de lisière**, chacune défaite par un outil : le **rongeur-racine** vit
  sous terre et seule la houe l'atteint ; le **corbeau voleur** vole par-dessus tout et craint
  la faux (arc de 3 cases) ; le **ronceau** a une écorce que la hache brise pour de bon (et le
  repousse) ; le **scarabée-galet** a une carapace que la pioche perce (et l'étourdit).
- **Vague nocturne** : à 20 h, s'il y a des cultures et quelqu'un d'éveillé, des nuisibles
  sortent de la forêt et mangent les cultures. Plus il y a de champs et de jours, plus ils sont
  nombreux ; les ronceaux et scarabées s'y joignent à partir du jour 3. À l'aube, ils fuient.
  Une vallée vide n'est jamais attaquée.
- **Vie et mort** : 10 points de vie, rendus chaque matin ; manger une récolte ou une ration
  soigne. Tomber à 0 renvoie au lit, et **le sac et tout son contenu sont perdus** ; les outils
  et le **coffre de la maison** sont à l'abri. Quitter le jeu avec une créature à 2 cases ou
  moins compte comme tomber.

## Ce qu'on peut faire (phase 3)

- **Expérience et niveaux** : récolter, labourer, semer, arroser, couper, dessoucher, construire,
  combattre, nourrir et apprivoiser donnent de l'expérience. Chaque niveau donne un point de talent.
- **Constellation des Racines** (T) : 61 nœuds en quatre régions, **Terre** (cultures), **Sylve**
  (créatures), **Forge** (construction, énergie) et **Lame** (combat), reliées par des ponts. On part
  du cœur ; un point prend un nœud relié à ce qu'on a déjà. Petits nœuds, nœuds notables nommés, et
  deux **pierres angulaires** : **Main verte** (l'arrosoir n'arrose plus ; tes semis poussent sans eau
  un jour sur deux et à chaque pluie, ne fanent jamais et donnent le double) et **Meneur de meute**
  (tes outils ne blessent plus, tes créatures font trois fois plus de dégâts). Retirer un point coûte
  une **graine d'oubli** (une au départ, rarement trouvée en fauchant, et le pavot d'oubli en donne).
  L'écran se zoome à la molette et se déplace en glissant ; clic pour prendre, clic droit pour retirer.
- **Apprivoisement** : on gagne la confiance en nourrissant (un repas par heure de jeu). La
  **chèvre-lichen** (blé, fève) est paisible ; le **hibou-lanterne** (dent de rongeur, fève) ne sort que
  la nuit ; le **loup-ronce** (ration, navet, dent de rongeur) attaque : il faut d'abord l'affaiblir
  sans le tuer, il se rend sous un tiers de sa vie. Frapper une créature paisible lui fait tout
  oublier. Confiance pleine, elle s'attache à toi s'il y a un abri libre dans le domaine : **enclos**,
  **tanière** ou **nichoir** (à construire, B).
- **Rôles** : la chèvre laisse du **lait** dans son enclos chaque matin après un jour où tu l'as
  nourrie (E sur l'enclos) ; le loup **garde** sa tanière et attaque tout ce qui approche à 6 cases ;
  le hibou te **suit**, éclaire la nuit et attaque ce qui t'approche. Une seule créature te suit au
  départ (la Sylve en ajoute). Tu passes à travers tes créatures, tes coups ne les touchent jamais ;
  épuisée, une créature rentre se reposer jusqu'au matin. Elles sont sauvegardées avec la vallée.

Tous ces réglages sont dans `assets/data/contenu.json`, lu par le serveur (règles) et le client
(noms) : on équilibre sans toucher au code. Le serveur le lit au démarrage (`-content`). La
constellation est dans `assets/data/constellation.json` (nœuds, positions, liens, modificateurs),
validée par le serveur : chaque nœud doit être relié au cœur et n'utiliser que des modificateurs connus.

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
  Calques attendus : `sol` (tuiles, propriétés booléennes `bloquant`, `cultivable`, `eau` et
  `lisiere`, d'où viennent les créatures), `ressources` (objets de type `arbre`, `rocher` ou
  `herbes`) et `reperes` (objets `apparition`, `lit` (la case sous le lit doit être libre),
  `coffre`, et un rectangle `domaine` : le domaine de départ).
- **Art original.** Tout l'art actuel est provisoire et dessiné par `genassets`. Une fois la carte
  retouchée dans Tiled, ne relance plus `genassets`, qui l'écraserait.

## Tests

```sh
cd server && go test -race ./...
cd client && npm test && npm run build
```

`TestNightIsTenseButWinnable` joue des nuits entières sur la vraie carte (critère de sortie de
la phase 2) : un défenseur qui utilise les bons outils doit gagner la plupart des nuits, et un
champ laissé seul doit être dévoré. `go test -short` le saute.

Le test PostgreSQL ne tourne que si `LV_TEST_DATABASE_URL` pointe vers une base jetable
(la CI en fournit une).

## Pas encore là

Saisons qui comptent pour les cultures (le calendrier défile, mais tout pousse en toute saison),
qualité des récoltes, vente, forge et amélioration des outils, arrosoir et fronde au combat,
bestiaire, humeur et élevage des créatures, montures, créatures qui voyagent avec leur joueur,
comptes, visites et permissions sur les bâtiments et le coffre (phase 4), personnalisation du
personnage. La confiance d'une créature sauvage n'est pas sauvegardée (seules les créatures
apprivoisées le sont).
