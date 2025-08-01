# Build Instructions

## GitHub Actions CI/CD Pipeline

Ce projet inclut deux workflows GitHub Actions pour automatiser les builds :

### 1. Workflow Principal (`build-windows.yml`)
- **Triggers**: Push sur `main`/`develop`, Pull Requests, Releases
- **Plateformes**: Windows, Linux, macOS (ARM64 et AMD64)
- **Fonctionnalités**:
  - Build cross-platform automatique
  - Upload des artifacts (binaries)
  - Création automatique de releases avec archives

### 2. Workflow Windows Only (`windows-only.yml`)
- **Triggers**: Push sur `main`, déclenchement manuel
- **Plateforme**: Windows uniquement (plus rapide)
- **Fonctionnalités**:
  - Build optimisé pour Windows
  - Package portable avec README
  - Tests basiques de l'exécutable

## Utilisation des Workflows

### Déclenchement Automatique
Les builds se lancent automatiquement lors de :
- Push sur la branche `main`
- Création d'une Pull Request
- Création d'une Release GitHub

### Déclenchement Manuel
Pour lancer le build Windows manuellement :
1. Aller dans l'onglet "Actions" de votre repo GitHub
2. Sélectionner "Windows Build Only"
3. Cliquer sur "Run workflow"

## Build Local (Windows)

### Prérequis
- Go 1.21+
- Node.js 18+
- Wails CLI v2

### Script Automatisé
```powershell
# Build basique
.\build-windows.ps1

# Build avec nettoyage
.\build-windows.ps1 -Clean

# Build de développement (avec debug)
.\build-windows.ps1 -Dev

# Build pour une autre plateforme
.\build-windows.ps1 -Platform "windows/386"
```

### Build Manuel
```bash
# Installer les dépendances frontend
cd frontend
npm install

# Retourner à la racine
cd ..

# Build avec Wails
wails build -clean -platform windows/amd64
```

## Configuration

### `wails.json`
Configuration principale du projet Wails avec :
- Métadonnées de l'application (nom, version, copyright)
- Configuration du build (mode production, optimisations)
- Hooks pré/post build (actuellement vides)

### Variables d'Environnement GitHub
Aucune variable secrète requise pour les builds basiques.

Pour des fonctionnalités avancées, vous pouvez ajouter :
- `GITHUB_TOKEN` : Automatiquement fourni par GitHub Actions

## Artifacts

### Builds Automatiques
Les artifacts sont automatiquement uploadés et disponibles pendant :
- **7 jours** : Exécutables de développement
- **30 jours** : Packages portables et releases

### Structure des Artifacts
```
card-scraper-windows-amd64/
├── card-scraper.exe          # Exécutable principal
└── README.txt               # Instructions utilisateur
```

## Optimisations Build

### Flags de Compilation
- `-ldflags "-s -w"` : Supprime les symboles de debug et réduit la taille
- `-clean` : Nettoie les builds précédents
- Mode production activé par défaut

### Cache
- Cache Go modules automatiquement
- Cache npm dependencies avec `package-lock.json`

## Résolution de Problèmes

### Build Fails
1. Vérifier les prérequis (Go, Node.js, Wails)
2. Nettoyer le cache : `wails build -clean`
3. Vérifier les logs dans l'onglet Actions

### Performance
- Les builds cross-platform prennent ~5-10 minutes
- Le build Windows uniquement prend ~3-5 minutes
- Utilisez le cache pour accélérer les builds répétés

### Taille des Binaires
- Executable Windows : ~40-60 MB
- Réduction possible avec UPX (non inclus par défaut)

## Release Process

### Automatique
1. Créer un tag : `git tag v1.0.0`
2. Push le tag : `git push origin v1.0.0`
3. Créer une Release sur GitHub
4. Les binaries sont automatiquement attachés

### Manuel
1. Build local avec `.\build-windows.ps1`
2. Upload manuel des artifacts dans la Release GitHub