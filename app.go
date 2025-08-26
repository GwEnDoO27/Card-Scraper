package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	_ "github.com/mattn/go-sqlite3"
)

type App struct {
	ctx context.Context
	db  *sql.DB
}

type Card struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Set         string  `json:"set_name"`
	Rarity      string  `json:"rarity"`
	Price       string  `json:"price"`
	PriceNum    float64 `json:"price_num"`
	ImageURL    string  `json:"image_url"`
	CardURL     string  `json:"card_url"`
	Type        string  `json:"type"` // "collection" ou "wishlist"
	AddedAt     string  `json:"added_at"`
	LastUpdated string  `json:"last_updated"`
	// Nouvelles propriétés détaillées
	Quality     string `json:"quality"`      // Qualité sélectionnée (NM, LP, etc.)
	Language    string `json:"language"`     // Langue sélectionnée
	Edition     bool   `json:"edition"`      // Première édition ou non
	TotalOffers int    `json:"total_offers"` // Nombre total d'offres trouvées
}

type AddCardRequest struct {
	URL      string `json:"url"`
	Type     string `json:"type"`     // "collection" ou "wishlist"
	Quality  string `json:"quality"`  // "NM", "LP", "MP", "HP", "PO"
	Language string `json:"language"` // "Français", "English", etc.
	Edition  bool   `json:"edition"`  // true pour première édition
}

func NewApp() *App {
	db, err := sql.Open("sqlite3", "./cardmarket_app.db")
	if err != nil {
		log.Fatal(err)
	}

	// Créer les tables
	createTables := `
	CREATE TABLE IF NOT EXISTS cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		set_name TEXT,
		rarity TEXT,
		price TEXT,
		price_num REAL,
		image_url TEXT,
		card_url TEXT UNIQUE,
		type TEXT NOT NULL, -- 'collection' ou 'wishlist'
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_cards_type ON cards(type);
	CREATE INDEX IF NOT EXISTS idx_cards_url ON cards(card_url);
	`

	_, err = db.Exec(createTables)
	if err != nil {
		log.Fatal(err)
	}

	// Ajouter les nouvelles colonnes une par une, en gérant les erreurs
	newColumns := []string{
		"ALTER TABLE cards ADD COLUMN quality TEXT DEFAULT ''",
		"ALTER TABLE cards ADD COLUMN language TEXT DEFAULT ''",
		"ALTER TABLE cards ADD COLUMN edition BOOLEAN DEFAULT FALSE",
		"ALTER TABLE cards ADD COLUMN total_offers INTEGER DEFAULT 0",
	}

	for _, query := range newColumns {
		_, err = db.Exec(query)
		if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			log.Printf("Erreur lors de l'ajout de colonne: %v", err)
		}
	}

	return &App{db: db}
}

func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
}

// Ajouter une nouvelle carte
func (a *App) AddCard(req AddCardRequest) (*Card, error) {
	log.Printf("Ajout d'une carte: URL=%s, Type=%s", req.URL, req.Type)

	// Vérifier si la carte existe déjà
	existingCard, err := a.getCardByURL(req.URL)
	if err == nil {
		// La carte existe déjà
		if existingCard.Type == req.Type {
			return nil, fmt.Errorf("cette carte est déjà dans votre %s", req.Type)
		} else {
			// Déplacer la carte d'un type à l'autre
			err = a.moveCard(existingCard.ID, req.Type)
			if err != nil {
				return nil, err
			}
			existingCard.Type = req.Type
			return existingCard, nil
		}
	}

	// Scraper les informations de la carte
	cardInfo, err := a.scrapeCardInfo(req.URL, req)
	if err != nil {
		log.Printf("❌ Erreur scraping: %v", err)
		
		// Messages d'erreur spécifiques selon le contexte
		if strings.Contains(err.Error(), "impossible de se connecter au navigateur") {
			if runtime.GOOS == "windows" {
				return nil, fmt.Errorf("impossible d'accéder au navigateur. Sur Windows: 1) Vérifiez que Chrome/Edge est installé, 2) Ajoutez l'application aux exclusions antivirus, 3) Désactivez temporairement Windows Defender si nécessaire")
			}
			return nil, fmt.Errorf("impossible d'accéder au navigateur: %v", err)
		}
		
		if strings.Contains(err.Error(), "aucune carte correspondant aux critères") ||
			strings.Contains(err.Error(), "impossible d'extraire les offres") {
			return nil, fmt.Errorf("carte non trouvée avec les critères spécifiés (qualité: %s, langue: %s, édition: %t). Aucune carte similaire disponible",
				req.Quality, req.Language, req.Edition)
		}
		
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, fmt.Errorf("timeout lors de l'accès à CardMarket. Vérifiez votre connexion internet et réessayez")
		}
		
		return nil, fmt.Errorf("erreur lors du scraping: %v", err)
	}

	// Sauvegarder en base
	card := &Card{
		Name:        cardInfo.Name,
		Set:         cardInfo.Set,
		Rarity:      cardInfo.Rarity,
		Price:       cardInfo.Price,
		PriceNum:    cardInfo.PriceNum,
		ImageURL:    cardInfo.ImageURL,
		CardURL:     req.URL,
		Type:        req.Type,
		AddedAt:     time.Now().Format("2006-01-02 15:04:05"),
		LastUpdated: time.Now().Format("2006-01-02 15:04:05"),
		Quality:     req.Quality,
		Language:    req.Language,
		Edition:     req.Edition,
		TotalOffers: len(cardInfo.Offers),
	}

	result, err := a.db.Exec(`
		INSERT INTO cards (name, set_name, rarity, price, price_num, image_url, card_url, type, added_at, last_updated, quality, language, edition, total_offers)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, card.Name, card.Set, card.Rarity, card.Price, card.PriceNum, card.ImageURL, card.CardURL, card.Type, card.AddedAt, card.LastUpdated, card.Quality, card.Language, card.Edition, card.TotalOffers)

	if err != nil {
		return nil, fmt.Errorf("erreur sauvegarde: %v", err)
	}

	id, _ := result.LastInsertId()
	card.ID = int(id)

	return card, nil
}

func (a *App) Sumprice() (float64, error) {
	var totalPrice float64
	err := a.db.QueryRow(`
		SELECT COALESCE(SUM(price_num), 0)
		FROM cards
	`).Scan(&totalPrice)
	if err != nil {
		return 0.0, err
	}

	return totalPrice, nil
}

// Rescraper toutes les cartes pour mettre à jour les prix
func (a *App) RescrapAllCards() (map[string]interface{}, error) {
	log.Println("🔄 Début du rescrap de toutes les cartes...")
	
	// Récupérer toutes les cartes
	rows, err := a.db.Query(`
		SELECT id, card_url, type, quality, language, edition
		FROM cards
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des cartes: %v", err)
	}
	defer rows.Close()

	stats := map[string]interface{}{
		"total_cards": 0,
		"updated": 0,
		"errors": 0,
		"error_details": []string{},
	}

	var cards []struct {
		ID       int
		URL      string
		Type     string
		Quality  string
		Language string
		Edition  bool
	}

	// Collecter toutes les cartes
	for rows.Next() {
		var card struct {
			ID       int
			URL      string
			Type     string
			Quality  string
			Language string
			Edition  bool
		}
		err := rows.Scan(&card.ID, &card.URL, &card.Type, &card.Quality, &card.Language, &card.Edition)
		if err != nil {
			log.Printf("Erreur lors de la lecture de la carte: %v", err)
			continue
		}
		cards = append(cards, card)
	}

	stats["total_cards"] = len(cards)
	log.Printf("📊 %d cartes à rescraper", len(cards))

	// Rescraper chaque carte
	for i, card := range cards {
		log.Printf("🔄 Rescrap carte %d/%d: ID=%d", i+1, len(cards), card.ID)
		
		// Créer la requête pour rescraper
		req := AddCardRequest{
			URL:      card.URL,
			Type:     card.Type,
			Quality:  card.Quality,
			Language: card.Language,
			Edition:  card.Edition,
		}

		// Scraper les nouvelles informations
		cardInfo, err := a.scrapeCardInfo(card.URL, req)
		if err != nil {
			errorMsg := fmt.Sprintf("Carte ID %d: %v", card.ID, err)
			log.Printf("❌ %s", errorMsg)
			stats["errors"] = stats["errors"].(int) + 1
			if errorDetails, ok := stats["error_details"].([]string); ok {
				stats["error_details"] = append(errorDetails, errorMsg)
			}
			continue
		}

		// Mettre à jour la carte en base
		_, err = a.db.Exec(`
			UPDATE cards 
			SET name = ?, set_name = ?, rarity = ?, price = ?, price_num = ?, 
			    image_url = ?, last_updated = CURRENT_TIMESTAMP
			WHERE id = ?
		`, cardInfo.Name, cardInfo.Set, cardInfo.Rarity, cardInfo.Price, 
		   cardInfo.PriceNum, cardInfo.ImageURL, card.ID)

		if err != nil {
			errorMsg := fmt.Sprintf("Carte ID %d: erreur sauvegarde %v", card.ID, err)
			log.Printf("❌ %s", errorMsg)
			stats["errors"] = stats["errors"].(int) + 1
			if errorDetails, ok := stats["error_details"].([]string); ok {
				stats["error_details"] = append(errorDetails, errorMsg)
			}
			continue
		}

		stats["updated"] = stats["updated"].(int) + 1
		log.Printf("✅ Carte ID %d mise à jour: %s - %s", card.ID, cardInfo.Price, cardInfo.Name)
	}

	log.Printf("🎉 Rescrap terminé: %d/%d cartes mises à jour, %d erreurs", 
		stats["updated"], stats["total_cards"], stats["errors"])
	
	return stats, nil
}

// Récupérer toutes les cartes d'un type
func (a *App) GetCards(cardType string) ([]Card, error) {
	rows, err := a.db.Query(`
		SELECT id, name, set_name, rarity, price, price_num, image_url, card_url, type, added_at, last_updated,
		       COALESCE(quality, '') as quality, COALESCE(language, '') as language, 
		       COALESCE(edition, FALSE) as edition, COALESCE(total_offers, 0) as total_offers
		FROM cards
		WHERE type = ?
		ORDER BY added_at DESC
	`, cardType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []Card
	for rows.Next() {
		var card Card
		err := rows.Scan(&card.ID, &card.Name, &card.Set, &card.Rarity, &card.Price, &card.PriceNum,
			&card.ImageURL, &card.CardURL, &card.Type, &card.AddedAt, &card.LastUpdated,
			&card.Quality, &card.Language, &card.Edition, &card.TotalOffers)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}

	return cards, nil
}

// Supprimer une carte
func (a *App) DeleteCard(cardID int) error {
	_, err := a.db.Exec("DELETE FROM cards WHERE id = ?", cardID)
	return err
}

// Déplacer une carte d'une liste à l'autre
func (a *App) MoveCard(cardID int, newType string) error {
	return a.moveCard(cardID, newType)
}

func (a *App) moveCard(cardID int, newType string) error {
	_, err := a.db.Exec(`
		UPDATE cards 
		SET type = ?, last_updated = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, newType, cardID)
	return err
}

// Récupérer les statistiques
func (a *App) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Compter les cartes par type
	var collectionCount, wishlistCount int
	var collectionValue, wishlistValue float64

	err := a.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(price_num), 0) FROM cards WHERE type = 'collection'").Scan(&collectionCount, &collectionValue)
	if err != nil {
		return nil, err
	}

	err = a.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(price_num), 0) FROM cards WHERE type = 'wishlist'").Scan(&wishlistCount, &wishlistValue)
	if err != nil {
		return nil, err
	}

	stats["collection_count"] = collectionCount
	stats["wishlist_count"] = wishlistCount
	stats["collection_value"] = collectionValue
	stats["wishlist_value"] = wishlistValue
	stats["total_cards"] = collectionCount + wishlistCount
	stats["total_value"] = collectionValue + wishlistValue

	return stats, nil
}

// GetSystemInfo retourne des informations système pour debug
func (a *App) GetSystemInfo() map[string]interface{} {
	info := map[string]interface{}{
		"os":           runtime.GOOS,
		"architecture": runtime.GOARCH,
		"go_version":   runtime.Version(),
	}

	if runtime.GOOS == "windows" {
		// Informations spécifiques Windows
		browserPath := (&App{}).findWindowsBrowser()
		info["browser_found"] = browserPath != ""
		info["browser_path"] = browserPath
		
		// Variables d'environnement Windows importantes
		info["program_files"] = os.Getenv("ProgramFiles")
		info["program_files_x86"] = os.Getenv("ProgramFiles(x86)")
		info["local_appdata"] = os.Getenv("LOCALAPPDATA")
		info["user_profile"] = os.Getenv("USERPROFILE")
	}

	return info
}

// Fonctions utilitaires internes
func (a *App) getCardByURL(url string) (*Card, error) {
	var card Card
	err := a.db.QueryRow(`
		SELECT id, name, set_name, rarity, price, price_num, image_url, card_url, type, added_at, last_updated,
		       COALESCE(quality, '') as quality, COALESCE(language, '') as language, 
		       COALESCE(edition, FALSE) as edition, COALESCE(total_offers, 0) as total_offers
		FROM cards WHERE card_url = ?
	`, url).Scan(&card.ID, &card.Name, &card.Set, &card.Rarity, &card.Price, &card.PriceNum,
		&card.ImageURL, &card.CardURL, &card.Type, &card.AddedAt, &card.LastUpdated,
		&card.Quality, &card.Language, &card.Edition, &card.TotalOffers)
	return &card, err
}

func (a *App) getCardByID(id int) (*Card, error) {
	var card Card
	err := a.db.QueryRow(`
		SELECT id, name, set_name, rarity, price, price_num, image_url, card_url, type, added_at, last_updated,
		       COALESCE(quality, '') as quality, COALESCE(language, '') as language, 
		       COALESCE(edition, FALSE) as edition, COALESCE(total_offers, 0) as total_offers
		FROM cards WHERE id = ?
	`, id).Scan(&card.ID, &card.Name, &card.Set, &card.Rarity, &card.Price, &card.PriceNum,
		&card.ImageURL, &card.CardURL, &card.Type, &card.AddedAt, &card.LastUpdated,
		&card.Quality, &card.Language, &card.Edition, &card.TotalOffers)
	return &card, err
}

type ScrapedCardInfo struct {
	Name     string
	Set      string
	Rarity   string
	Price    string
	PriceNum float64
	ImageURL string
	Offers   []CardOffer
}

type CardOffer struct {
	Mint     string  `json:"mint"`
	Language string  `json:"language"`
	Edition  bool    `json:"edition"`
	Price    string  `json:"price"`
	PriceNum float64 `json:"price_num"`
	Rarity   string  `json:"rarity"`
	SetName  string  `json:"set_name"`
}

// getChromeOptions retourne les options Chrome optimisées selon l'OS
func (a *App) getChromeOptions() []chromedp.ExecAllocatorOption {
	// Mode compatibilité antivirus : options moins agressives
	opts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-features", "VizDisplayCompositor"),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", false), // Important: laisser false
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-web-security", false), // Important: sécurité activée
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	}

	// Configuration spécifique à Windows - Mode compatibilité antivirus
	if runtime.GOOS == "windows" {
		log.Println("🪟 Mode Windows - Configuration sécurisée antivirus")
		
		// User-Agent Windows standard
		opts = append(opts, chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"))
		
		// Options Windows avec compatibilité antivirus
		opts = append(opts, 
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("disable-gpu-sandbox", true),
			chromedp.Flag("disable-software-rasterizer", true),
			chromedp.Flag("disable-dev-shm-usage", true), // Évite les problèmes de mémoire partagée
			chromedp.Flag("remote-debugging-port", "0"), // Désactive le debugging distant
			chromedp.Flag("disable-logging", true),
			chromedp.Flag("log-level", "3"), // Erreurs seulement
			chromedp.Flag("silent", true),
		)

		// Mode sécurisé : ne pas utiliser --no-sandbox sur Windows par défaut
		// L'antivirus préfère que le sandbox soit activé
		
		// Chercher Chrome ou Edge - préférer Edge sur Windows
		chromePath := a.findWindowsBrowserSecure()
		if chromePath != "" {
			log.Printf("🌐 Navigateur sécurisé trouvé: %s", filepath.Base(chromePath))
			opts = append([]chromedp.ExecAllocatorOption{chromedp.ExecPath(chromePath)}, opts...)
		} else {
			log.Println("⚠️  Aucun navigateur trouvé - mode de compatibilité")
			// En dernier recours, ajouter no-sandbox mais avec avertissement
			opts = append(opts, chromedp.Flag("no-sandbox", true))
		}
	} else {
		// Configuration macOS/Linux
		opts = append(opts, chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"))
		opts = append(opts, chromedp.Flag("no-sandbox", true))
	}

	return opts
}

// findWindowsBrowserSecure cherche un navigateur en privilégiant Edge pour la sécurité
func (a *App) findWindowsBrowserSecure() string {
	if runtime.GOOS != "windows" {
		return ""
	}

	// Ordre de préférence : Edge puis Chrome (Edge est moins suspect pour les antivirus)
	browsers := []string{
		// Microsoft Edge (priorité 1 - intégré à Windows)
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
		
		// Google Chrome (priorité 2)
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
	}

	for _, path := range browsers {
		if _, err := os.Stat(path); err == nil {
			// Vérifier que le fichier est accessible en lecture
			if file, err := os.Open(path); err == nil {
				file.Close()
				log.Printf("✅ Navigateur accessible: %s", filepath.Base(path))
				return path
			} else {
				log.Printf("⚠️  Navigateur trouvé mais non accessible: %s (%v)", filepath.Base(path), err)
			}
		}
	}

	return ""
}

// findWindowsBrowser cherche Chrome ou Edge sur Windows (méthode legacy)
func (a *App) findWindowsBrowser() string {
	return a.findWindowsBrowserSecure()
}

// testBrowserConnectionPatient teste la connexion avec patience pour Windows
func (a *App) testBrowserConnectionPatient(ctx context.Context) error {
	if runtime.GOOS == "windows" {
		log.Println("🔍 Test de connexion navigateur Windows (mode patient)...")
		
		// Timeout plus long pour Windows à cause des antivirus
		testCtx, testCancel := context.WithTimeout(ctx, 30*time.Second)
		defer testCancel()

		// Test progressif pour éviter les détections
		err := chromedp.Run(testCtx,
			// Attendre plus longtemps au démarrage
			chromedp.Sleep(3*time.Second),
			chromedp.Navigate("about:blank"),
			chromedp.Sleep(2*time.Second),
			chromedp.WaitVisible("body", chromedp.ByQuery),
		)

		if err != nil {
			return fmt.Errorf("connexion navigateur impossible: %v. Solutions: 1) Redémarrez l'app en tant qu'administrateur, 2) Ajoutez l'app aux exclusions antivirus, 3) Vérifiez qu'Edge/Chrome est installé", err)
		}

		log.Println("✅ Navigateur Windows connecté avec succès")
		return nil
	} else {
		return a.testBrowserConnection(ctx)
	}
}

// testBrowserConnection teste si le navigateur répond correctement
func (a *App) testBrowserConnection(ctx context.Context) error {
	log.Println("🔍 Test de connexion au navigateur...")
	
	// Créer un contexte avec timeout court pour le test
	testCtx, testCancel := context.WithTimeout(ctx, 10*time.Second)
	defer testCancel()

	// Test simple: naviguer vers about:blank
	err := chromedp.Run(testCtx,
		chromedp.Navigate("about:blank"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)

	if err != nil {
		return fmt.Errorf("échec connexion navigateur: %v", err)
	}

	log.Println("✅ Navigateur connecté avec succès")
	return nil
}

// scrapeWithRetries effectue le scraping avec plusieurs tentatives pour Windows
func (a *App) scrapeWithRetries(req AddCardRequest, ctx context.Context, url string) *CardOffer {
	log.Println("🔄 Mode Windows : scraping avec tentatives multiples...")
	
	attempts := []struct {
		delay    time.Duration
		loadMore bool
		name     string
	}{
		{2 * time.Second, false, "Tentative rapide"},
		{5 * time.Second, false, "Tentative standard"},
		{8 * time.Second, true, "Tentative avec chargement supplémentaire"},
		{12 * time.Second, true, "Tentative finale (mode patient)"},
	}

	for i, attempt := range attempts {
		log.Printf("🎯 %s (%d/%d)...", attempt.name, i+1, len(attempts))
		
		// Délai avant chaque tentative pour éviter la détection
		if i > 0 {
			log.Printf("⏳ Attente de %v avant la tentative...", attempt.delay)
			time.Sleep(attempt.delay)
		}

		result := a.launchLoopPatient(req.Quality, req.Language, req.Edition, attempt.loadMore, ctx, url)
		if result != nil {
			log.Printf("✅ Succès avec %s !", attempt.name)
			return result
		}
		
		log.Printf("❌ %s échouée, passage à la suivante...", attempt.name)
	}

	log.Println("❌ Toutes les tentatives ont échoué")
	return nil
}

func (a *App) scrapeCardInfo(url string, req AddCardRequest) (*ScrapedCardInfo, error) {
	log.Printf("🚀 Démarrage scraping pour: %s", url)

	// Configuration Chrome optimisée pour Windows
	opts := a.getChromeOptions()

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	// Créer le contexte avec logging pour debug
	ctx, ctxCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer ctxCancel()

	// Test de connectivité du navigateur avec mode patient pour Windows
	if err := a.testBrowserConnectionPatient(ctx); err != nil {
		return nil, fmt.Errorf("impossible de se connecter au navigateur: %v", err)
	}

	info := &ScrapedCardInfo{}

	// Mode Windows : tentatives multiples avec délais plus longs
	if runtime.GOOS == "windows" {
		result := a.scrapeWithRetries(req, ctx, url)
		if result == nil {
			return nil, fmt.Errorf("aucune carte correspondant aux critères qualité=%s, langue=%s, édition=%t après plusieurs tentatives", req.Quality, req.Language, req.Edition)
		}
		// Utiliser directement le résultat des tentatives multiples
		info.Offers = []CardOffer{*result}
		info.Price = result.Price
		info.PriceNum = result.PriceNum
	} else {
		// Mode standard pour macOS/Linux
		result := a.launchLoop(req.Quality, req.Language, req.Edition, false, ctx, url)
		if result == nil {
			log.Println("🔄 Première tentative échouée, essai avec chargement supplémentaire...")
			result = a.launchLoop(req.Quality, req.Language, req.Edition, true, ctx, url)
		}
		if result == nil {
			return nil, fmt.Errorf("aucune carte correspondant aux critères qualité=%s, langue=%s, édition=%t", req.Quality, req.Language, req.Edition)
		}
		info.Offers = []CardOffer{*result}
		info.Price = result.Price
		info.PriceNum = result.PriceNum
	}

	// Extraire les informations de base (nom, set, rareté)
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		log.Printf("Erreur navigation: %v", err)
	}

	// Extraire le nom
	var name string
	err = chromedp.Run(ctx, chromedp.Text("h1", &name, chromedp.ByQuery))
	if err != nil || name == "" {
		name = "Carte inconnue"
	}
	info.Name = strings.TrimSpace(name)

	// Extraire la rareté et le set depuis l'info-list-container
	var rarityFromPage, setFromPage string
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				var result = {rarity: '', set_name: ''};
				try {
					var infoContainer = document.querySelector('.info-list-container');
					if (infoContainer) {
						// Extraire la rareté - chercher le SVG avec data-bs-original-title
						var rarityElement = infoContainer.querySelector('svg[data-bs-original-title]');
						result.rarity = rarityElement ? rarityElement.getAttribute('data-bs-original-title') : '';
						
						// Extraire le nom du set - chercher le lien vers l'expansion
						var setElement = infoContainer.querySelector('a[href*="/Expansions/"]');
						result.set_name = setElement ? setElement.textContent.trim() : '';
					}
				} catch(e) {
					console.log('Erreur extraction:', e);
				}
				return result;
			})()
		`, &map[string]interface{}{}),
	)
	
	// Extraire les valeurs depuis le résultat JavaScript
	if err == nil {
		var pageInfo map[string]interface{}
		err = chromedp.Run(ctx,
			chromedp.Evaluate(`
				(function() {
					var result = {rarity: '', set_name: ''};
					try {
						var infoContainer = document.querySelector('.info-list-container');
						if (infoContainer) {
							var rarityElement = infoContainer.querySelector('svg[data-bs-original-title]');
							result.rarity = rarityElement ? rarityElement.getAttribute('data-bs-original-title') : '';
							
							var setElement = infoContainer.querySelector('a[href*="/Expansions/"]');
							result.set_name = setElement ? setElement.textContent.trim() : '';
						}
					} catch(e) {
						console.log('Erreur extraction:', e);
					}
					return result;
				})()
			`, &pageInfo),
		)
		
		if err == nil && pageInfo != nil {
			if rarity, ok := pageInfo["rarity"].(string); ok {
				rarityFromPage = strings.TrimSpace(rarity)
			}
			if setName, ok := pageInfo["set_name"].(string); ok {
				setFromPage = strings.TrimSpace(setName)
			}
		}
	}
	
	log.Printf("Informations extraites de la page: rareté='%s', set='%s'", rarityFromPage, setFromPage)

	// Utiliser les informations extraites, en priorité depuis la page principale
	if setFromPage != "" {
		info.Set = setFromPage
		result.SetName = setFromPage // Mettre à jour aussi dans result pour les logs
	} else if result.SetName != "" {
		info.Set = result.SetName
	} else {
		info.Set = "Set inconnu"
	}
	
	if rarityFromPage != "" {
		info.Rarity = rarityFromPage
		result.Rarity = rarityFromPage // Mettre à jour aussi dans result pour les logs
	} else if result.Rarity != "" {
		info.Rarity = result.Rarity
	} else {
		info.Rarity = "Rareté inconnue"
	}
	
	info.Offers = []CardOffer{*result}

	// Utiliser la carte trouvée
	info.Price = result.Price
	info.PriceNum = result.PriceNum
	log.Printf("✅ Offre sélectionnée: %s (mint: %s, langue: %s, edition: %t, rarity: %s, set: %s)",
		result.Price, result.Mint, result.Language, result.Edition, result.Rarity, result.SetName)

	return info, nil
}

func (a *App) parseSetAndRarity(description string) (string, string) {
	// Essayer d'extraire le set et la rareté depuis la description
	// Format typique: "Set Name - Rarity"
	parts := strings.Split(description, "-")
	if len(parts) >= 2 {
		set := strings.TrimSpace(parts[0])
		rarity := strings.TrimSpace(parts[1])
		return set, rarity
	}
	return "Set inconnu", "Rareté inconnue"
}

func (a *App) extractNumericPrice(priceText string) float64 {
	// Extraire le nombre du texte du prix
	// Gère les formats: "3,50 €", "15.000,00€", "1234.56€", etc.
	
	// Regex pour capturer les nombres avec séparateurs de milliers et décimales
	re := regexp.MustCompile(`(\d{1,3}(?:[.,]\d{3})*(?:[.,]\d{1,2})?)`)
	matches := re.FindStringSubmatch(priceText)
	
	if len(matches) > 1 {
		priceStr := matches[1]
		
		// Déterminer le format du prix
		if strings.Contains(priceStr, ".") && strings.Contains(priceStr, ",") {
			// Format européen: 15.000,50 (point = milliers, virgule = décimales)
			// Supprimer les points (milliers) et remplacer virgule par point
			priceStr = strings.ReplaceAll(priceStr, ".", "")
			priceStr = strings.Replace(priceStr, ",", ".", 1)
		} else if strings.Count(priceStr, ".") == 1 {
			// Vérifier si c'est un séparateur de milliers ou de décimales
			parts := strings.Split(priceStr, ".")
			if len(parts) == 2 && len(parts[1]) == 3 && !strings.Contains(priceText, ",") {
				// Probablement un séparateur de milliers: 15.000
				priceStr = strings.ReplaceAll(priceStr, ".", "")
			}
			// Sinon c'est probablement des décimales: 15.50
		} else if strings.Contains(priceStr, ",") {
			// Format avec virgule comme séparateur décimal: 15,50
			priceStr = strings.Replace(priceStr, ",", ".", 1)
		}
		
		if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
			log.Printf("Prix extrait: '%s' -> %f", priceText, price)
			return price
		} else {
			log.Printf("Erreur conversion prix: '%s' -> '%s' : %v", priceText, priceStr, err)
		}
	}
	
	log.Printf("Impossible d'extraire le prix de: '%s'", priceText)
	return 0.0
}

// getPage configure et lance le navigateur Chrome
func (a *App) getPage(moreLoad bool, ctx context.Context, url string) error {
	// Naviguer vers la page
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("erreur lors de la navigation: %v", err)
	}

	// Attendre que Cloudflare finisse
	err = chromedp.Run(ctx,
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		log.Printf("Erreur lors de l'attente: %v\n", err)
	}

	log.Println("Protection Cloudflare contournée")

	// Fermer la bannière de cookies avec timeout
	log.Println("Tentative de fermeture de la bannière cookies...")

	// Créer un contexte avec timeout pour éviter le blocage
	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 10*time.Second)
	defer cancelTimeout()

	// Essayer plusieurs sélecteurs possibles avec timeout
	cookieSelectors := []string{
		"#denyAll",
		"#acceptAll",
		"[data-testid='cookie-banner-deny']",
		"[data-testid='cookie-banner-accept']",
		"button[class*='cookie'][class*='deny']",
		"button[class*='cookie'][class*='decline']",
		"//button[contains(text(), 'Refuser')]",
		"//button[contains(text(), 'Accepter')]",
		"//button[contains(text(), 'Reject')]",
		"//button[contains(text(), 'Accept')]",
	}

	cookieHandled := false
	for _, selector := range cookieSelectors {
		if strings.HasPrefix(selector, "//") {
			// XPath selector
			err = chromedp.Run(ctxTimeout,
				chromedp.Sleep(1*time.Second),
				chromedp.Click(selector, chromedp.BySearch),
			)
		} else {
			// CSS selector
			err = chromedp.Run(ctxTimeout,
				chromedp.Sleep(1*time.Second),
				chromedp.Click(selector, chromedp.ByQuery),
			)
		}

		if err == nil {
			log.Printf("Bannière cookies fermée avec le sélecteur: %s\n", selector)
			cookieHandled = true
			break
		}
	}

	if !cookieHandled {
		log.Println("Aucune bannière cookies trouvée ou déjà fermée - continuons...")
		// Attendre un peu au cas où il y aurait encore des éléments qui se chargent
		chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	}

	if moreLoad {
		log.Println("Tentative de chargement de contenu supplémentaire...")

		// Créer un contexte avec timeout pour le Load More
		ctxLoadMore, cancelLoadMore := context.WithTimeout(ctx, 15*time.Second)
		defer cancelLoadMore()

		// Faire défiler vers le bas
		err = chromedp.Run(ctxLoadMore,
			chromedp.Sleep(3*time.Second),
			chromedp.Evaluate("window.scrollTo(0, document.body.scrollHeight);", nil),
			chromedp.Sleep(2*time.Second),
		)
		if err != nil {
			log.Printf("Erreur lors du défilement: %v\n", err)
		} else {
			log.Println("Défilement vers le bas effectué")
		}

		// Vérifier si le bouton Load More existe et est visible
		var buttonExists bool
		err = chromedp.Run(ctxLoadMore,
			chromedp.Evaluate(`
				(function() {
					var btn = document.getElementById('loadMoreButton');
					return btn !== null && btn.offsetParent !== null;
				})()
			`, &buttonExists),
		)

		if err != nil {
			log.Printf("Erreur lors de la vérification du bouton Load More: %v\n", err)
		} else if buttonExists {
			log.Println("Bouton Load More détecté, tentative de clic...")

			// Chercher et cliquer sur le bouton "Load More"
			err = chromedp.Run(ctxLoadMore,
				chromedp.Evaluate("document.getElementById('loadMoreButton').scrollIntoView({behavior: 'smooth', block: 'center'});", nil),
				chromedp.Sleep(2*time.Second),
				chromedp.Evaluate("document.getElementById('loadMoreButton').click();", nil),
				chromedp.Sleep(5*time.Second), // Attendre plus longtemps pour le chargement
			)
			if err != nil {
				log.Printf("Erreur lors du clic sur 'Load More': %v\n", err)
			} else {
				log.Println("Bouton 'Load More' cliqué avec succès")
			}
		} else {
			log.Println("Bouton Load More non trouvé ou pas visible")
		}
	}

	return nil
}

// getInfos extrait les informations des cartes de la page
func (a *App) getInfos(ctx context.Context) ([]CardOffer, error) {
	log.Println("=== DÉBUT GETINFOS ===")

	var res []CardOffer

	// Attendre que la page se charge avec timeout
	log.Println("Attente du chargement complet de la page...")
	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	defer cancelTimeout()

	err := chromedp.Run(ctxTimeout,
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'attente de la page: %v", err)
	}
	log.Println("Page chargée")

	// Debug: afficher le titre de la page et l'URL
	var title, currentURL string
	err = chromedp.Run(ctx,
		chromedp.Title(&title),
		chromedp.Location(&currentURL),
	)
	if err == nil {
		log.Printf("Titre de la page: %s\n", title)
		log.Printf("URL actuelle: %s\n", currentURL)
	}

	// Debug: compter les éléments avec différents sélecteurs
	log.Println("=== DEBUGGING SELECTORS ===")
	possibleSelectors := []string{"article-row", "row", "product-row", "item-row", "offer-row"}

	for _, selector := range possibleSelectors {
		var count int
		err = chromedp.Run(ctx,
			chromedp.Evaluate(fmt.Sprintf("document.getElementsByClassName('%s').length", selector), &count),
		)
		if err == nil {
			log.Printf("Classe '%s': %d éléments trouvés\n", selector, count)
		}
	}

	// Compter le nombre total de divs
	var totalDivs int
	err = chromedp.Run(ctx,
		chromedp.Evaluate("document.getElementsByTagName('div').length", &totalDivs),
	)
	if err == nil {
		log.Printf("Total divs sur la page: %d\n", totalDivs)
	}

	// Vérifier si on est bien sur la bonne page
	var pageContent string
	err = chromedp.Run(ctx,
		chromedp.Evaluate("document.body.innerText.substring(0, 500)", &pageContent),
	)
	if err == nil {
		log.Printf("Début du contenu de la page: %s...\n", strings.ReplaceAll(pageContent, "\n", " "))
	}

	// Obtenir les données des cartes
	log.Println("Recherche des éléments article-row...")
	var rowsCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate("document.getElementsByClassName('article-row').length", &rowsCount),
	)
	if err != nil {
		return nil, fmt.Errorf("erreur lors du comptage des lignes: %v", err)
	}

	log.Printf("Nombre de lignes article-row trouvées: %d\n", rowsCount)

	if rowsCount == 0 {
		// Essayer d'autres sélecteurs possibles
		alternativeSelectors := []string{
			"tr[data-product]",
			".product-row",
			".item-row",
			"[class*='article']",
			"[class*='product']",
		}

		for _, altSelector := range alternativeSelectors {
			var altCount int
			err = chromedp.Run(ctx,
				chromedp.Evaluate(fmt.Sprintf("document.querySelectorAll('%s').length", altSelector), &altCount),
			)
			if err == nil && altCount > 0 {
				log.Printf("Sélecteur alternatif '%s': %d éléments trouvés\n", altSelector, altCount)
			}
		}

		return res, nil // Retourner une liste vide plutôt qu'une erreur
	}

	// Traiter chaque ligne
	for i := 0; i < rowsCount; i++ {
		log.Printf("Traitement de la carte %d/%d...\n", i+1, rowsCount)

		var cardData map[string]interface{}

		// Extraire les informations de chaque carte via JavaScript
		err = chromedp.Run(ctx,
			chromedp.Evaluate(fmt.Sprintf(`
				(function() {
					var rows = document.getElementsByClassName('article-row');
					var row = rows[%d];
					if (!row) return null;
					
					var result = {};
					
					try {
						// Mint condition
						var mintEl = row.querySelector('.product-attributes .badge');
						result.mint = mintEl ? mintEl.textContent.trim() : '';
						
						// Langue
						var langEl = row.querySelector('.product-attributes .icon');
						result.langue = langEl ? (langEl.getAttribute('data-original-title') || langEl.getAttribute('title') || '') : '';
						
						// Edition
						var editionEl = row.querySelector('.product-attributes .st_SpecialIcon');
						result.edition = editionEl ? true : false;
						
						// Price
						var priceEl = row.querySelector('.price-container');
						result.price = priceEl ? priceEl.textContent.trim() : '';
						
						result.success = true;
					} catch(e) {
						result.error = e.toString();
						result.success = false;
					}			
						
					// Extraire rareté et set depuis les informations de la carte
					try {
						var infoContainer = document.querySelector('.info-list-container');
						if (infoContainer) {
							// Extraire la rareté - chercher le SVG avec data-bs-original-title
							var rarityElement = infoContainer.querySelector('svg[data-bs-original-title]');
							result.rarity = rarityElement ? rarityElement.getAttribute('data-bs-original-title') : '';
							
							// Extraire le nom du set - chercher le lien vers l'expansion
							var setElement = infoContainer.querySelector('a[href*="/Expansions/"]');
							result.set_name = setElement ? setElement.textContent.trim() : '';
						}
					} catch(e) {
						result.rarity = '';
						result.set_name = '';
					}

					return result;
				})()
			`, i), &cardData),
		)

		if err != nil {
			log.Printf("Erreur JavaScript lors de l'extraction de la carte %d: %v\n", i+1, err)
			continue
		}

		if cardData == nil {
			log.Printf("Carte %d: données null\n", i+1)
			continue
		}

		if success, ok := cardData["success"].(bool); !ok || !success {
			if errorMsg, ok := cardData["error"].(string); ok {
				log.Printf("Erreur dans l'extraction de la carte %d: %s\n", i+1, errorMsg)
			}
			continue
		}

		mint := ""
		langue := ""
		price := ""
		rarity := ""
		setName := ""
		edition := false

		if v, ok := cardData["mint"].(string); ok {
			mint = strings.TrimSpace(v)
		}
		if v, ok := cardData["langue"].(string); ok {
			langue = strings.TrimSpace(v)
		}
		if v, ok := cardData["price"].(string); ok {
			price = strings.TrimSpace(v)
		}
		if v, ok := cardData["rarity"].(string); ok {
			rarity = strings.TrimSpace(v)
		}
		if v, ok := cardData["set_name"].(string); ok {
			setName = strings.TrimSpace(v)
		}
		if v, ok := cardData["edition"].(bool); ok {
			edition = v
		}

		cardOffer := CardOffer{
			Mint:     mint,
			Language: langue,
			Edition:  edition,
			Price:    price,
			PriceNum: a.extractNumericPrice(price),
			Rarity:   rarity,
			SetName:  setName,
		}

		log.Printf("Carte %d extraite: mint='%s', langue='%s', edition=%t, price='%s', rarity='%s', set='%s'\n",
			i+1, cardOffer.Mint, cardOffer.Language, cardOffer.Edition, cardOffer.Price, cardOffer.Rarity, cardOffer.SetName)

		res = append(res, cardOffer)
	}

	log.Printf("=== FIN GETINFOS - %d cartes extraites ===\n", len(res))
	return res, nil
}

// findTheCard recherche une carte avec les critères spécifiés
func (a *App) findTheCard(données []CardOffer, quality, langue string, edition bool) *CardOffer {
	log.Printf("Recherche: mint='%s', langue='%s', edition=%t\n", quality, langue, edition)
	log.Printf("Nombre total de cartes à examiner: %d\n", len(données))

	for i, row := range données {
		log.Printf("Carte %d: mint='%s', langue='%s', edition=%t\n",
			i+1, row.Mint, row.Language, row.Edition)

		if row.Mint == quality && row.Language == langue && row.Edition == edition {
			log.Printf("Carte trouvée: %+v\n", row)
			return &row
		}
	}

	log.Println("Carte non trouvée, nouvelle tentative en cours...")
	return nil
}

// launchLoopPatient lance le processus de scraping avec délais étendus pour Windows
func (a *App) launchLoopPatient(quality, langue string, edition, load bool, ctx context.Context, url string) *CardOffer {
	// Mode patient avec délais plus longs
	err := a.getPagePatient(load, ctx, url)
	if err != nil {
		log.Printf("Erreur lors de l'initialisation de la page (mode patient): %v", err)
		return nil
	}

	res, err := a.getInfosPatient(ctx)
	if err != nil {
		log.Printf("Erreur lors de l'extraction des informations (mode patient): %v", err)
		return nil
	}

	card := a.findTheCard(res, quality, langue, edition)
	return card
}

// launchLoop lance le processus de scraping
func (a *App) launchLoop(quality, langue string, edition, load bool, ctx context.Context, url string) *CardOffer {
	err := a.getPage(load, ctx, url)
	if err != nil {
		log.Printf("Erreur lors de l'initialisation de la page: %v", err)
		return nil
	}

	res, err := a.getInfos(ctx)
	if err != nil {
		log.Printf("Erreur lors de l'extraction des informations: %v", err)
		return nil
	}

	card := a.findTheCard(res, quality, langue, edition)
	return card
}

// getPagePatient configure la page avec des délais plus longs pour Windows
func (a *App) getPagePatient(moreLoad bool, ctx context.Context, url string) error {
	log.Println("🐌 Mode patient - Navigation avec délais étendus...")
	
	// Navigation plus lente
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(5*time.Second), // Délai plus long
	)
	if err != nil {
		return fmt.Errorf("erreur lors de la navigation (mode patient): %v", err)
	}

	// Attendre encore plus longtemps pour Cloudflare
	log.Println("⏳ Attente prolongée pour Cloudflare...")
	err = chromedp.Run(ctx, chromedp.Sleep(8*time.Second))
	if err != nil {
		log.Printf("Erreur lors de l'attente prolongée: %v\n", err)
	}

	// Fermeture cookies avec timeouts plus longs
	log.Println("🍪 Fermeture cookies (mode patient)...")
	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	defer cancelTimeout()

	cookieSelectors := []string{
		"#denyAll", "#acceptAll",
		"[data-testid='cookie-banner-deny']",
		"button[class*='cookie'][class*='deny']",
		"//button[contains(text(), 'Refuser')]",
		"//button[contains(text(), 'Accept')]",
	}

	for _, selector := range cookieSelectors {
		err := chromedp.Run(ctxTimeout,
			chromedp.Sleep(2*time.Second), // Délai plus long entre chaque tentative
		)
		if err == nil {
			if strings.HasPrefix(selector, "//") {
				err = chromedp.Run(ctxTimeout, chromedp.Click(selector, chromedp.BySearch))
			} else {
				err = chromedp.Run(ctxTimeout, chromedp.Click(selector, chromedp.ByQuery))
			}
			if err == nil {
				log.Printf("✅ Cookies fermés avec: %s\n", selector)
				break
			}
		}
	}

	// Chargement supplémentaire avec délais étendus
	if moreLoad {
		log.Println("📄 Chargement supplémentaire (mode patient)...")
		ctxLoadMore, cancelLoadMore := context.WithTimeout(ctx, 30*time.Second)
		defer cancelLoadMore()

		err = chromedp.Run(ctxLoadMore,
			chromedp.Sleep(5*time.Second),
			chromedp.Evaluate("window.scrollTo(0, document.body.scrollHeight);", nil),
			chromedp.Sleep(5*time.Second),
		)
		if err != nil {
			log.Printf("Erreur défilement patient: %v\n", err)
		}

		// Load More avec délais étendus
		var buttonExists bool
		err = chromedp.Run(ctxLoadMore,
			chromedp.Evaluate(`document.getElementById('loadMoreButton') !== null`, &buttonExists),
		)
		if err == nil && buttonExists {
			err = chromedp.Run(ctxLoadMore,
				chromedp.Sleep(3*time.Second),
				chromedp.Evaluate("document.getElementById('loadMoreButton').click();", nil),
				chromedp.Sleep(10*time.Second), // Attente très longue
			)
			if err == nil {
				log.Println("✅ Load More cliqué (mode patient)")
			}
		}
	}

	return nil
}

// getInfosPatient extrait les informations avec des délais étendus
func (a *App) getInfosPatient(ctx context.Context) ([]CardOffer, error) {
	log.Println("🔍 Extraction patiente des informations...")

	var res []CardOffer

	// Attendre encore plus longtemps
	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 45*time.Second)
	defer cancelTimeout()

	err := chromedp.Run(ctxTimeout,
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(8*time.Second), // Délai très long
	)
	if err != nil {
		return nil, fmt.Errorf("erreur attente page (mode patient): %v", err)
	}

	// Compter les éléments avec délai
	log.Println("🔢 Comptage patient des éléments...")
	var rowsCount int
	err = chromedp.Run(ctx,
		chromedp.Sleep(3*time.Second),
		chromedp.Evaluate("document.getElementsByClassName('article-row').length", &rowsCount),
	)
	if err != nil {
		return nil, fmt.Errorf("erreur comptage patient: %v", err)
	}

	log.Printf("📊 Mode patient: %d lignes trouvées\n", rowsCount)

	if rowsCount == 0 {
		return res, nil
	}

	// Traiter chaque ligne avec délais
	for i := 0; i < rowsCount; i++ {
		log.Printf("🐌 Extraction patiente carte %d/%d...\n", i+1, rowsCount)

		// Délai entre chaque carte
		time.Sleep(1 * time.Second)

		var cardData map[string]interface{}
		err = chromedp.Run(ctx,
			chromedp.Evaluate(fmt.Sprintf(`
				(function() {
					var rows = document.getElementsByClassName('article-row');
					var row = rows[%d];
					if (!row) return null;
					
					var result = {};
					try {
						var mintEl = row.querySelector('.product-attributes .badge');
						result.mint = mintEl ? mintEl.textContent.trim() : '';
						
						var langEl = row.querySelector('.product-attributes .icon');
						result.langue = langEl ? (langEl.getAttribute('data-original-title') || langEl.getAttribute('title') || '') : '';
						
						var editionEl = row.querySelector('.product-attributes .st_SpecialIcon');
						result.edition = editionEl ? true : false;
						
						var priceEl = row.querySelector('.price-container');
						result.price = priceEl ? priceEl.textContent.trim() : '';
						
						result.success = true;
					} catch(e) {
						result.error = e.toString();
						result.success = false;
					}
					return result;
				})()
			`, i), &cardData),
		)

		if err != nil || cardData == nil {
			continue
		}

		if success, ok := cardData["success"].(bool); !ok || !success {
			continue
		}

		// Extraire les données comme avant
		mint, _ := cardData["mint"].(string)
		langue, _ := cardData["langue"].(string)
		price, _ := cardData["price"].(string)
		edition, _ := cardData["edition"].(bool)

		cardOffer := CardOffer{
			Mint:     strings.TrimSpace(mint),
			Language: strings.TrimSpace(langue),
			Edition:  edition,
			Price:    strings.TrimSpace(price),
			PriceNum: a.extractNumericPrice(strings.TrimSpace(price)),
		}

		res = append(res, cardOffer)
	}

	log.Printf("✅ Mode patient: %d cartes extraites\n", len(res))
	return res, nil
}

