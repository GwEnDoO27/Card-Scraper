package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
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
	Quality     string  `json:"quality"`     // Qualité sélectionnée (NM, LP, etc.)
	Language    string  `json:"language"`    // Langue sélectionnée
	Edition     bool    `json:"edition"`     // Première édition ou non
	TotalOffers int     `json:"total_offers"` // Nombre total d'offres trouvées
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
		// Vérifier si c'est une erreur de critères non trouvés
		if strings.Contains(err.Error(), "aucune carte correspondant aux critères") || 
		   strings.Contains(err.Error(), "impossible d'extraire les offres") {
			log.Printf("❌ Carte non ajoutée: %v", err)
			return nil, fmt.Errorf("carte non trouvée avec les critères spécifiés (qualité: %s, langue: %s, édition: %t). Aucune carte similaire disponible", 
				req.Quality, req.Language, req.Edition)
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
}

func (a *App) scrapeCardInfo(url string, req AddCardRequest) (*ScrapedCardInfo, error) {
	log.Printf("🚀 Démarrage scraping pour: %s", url)
	
	// Configuration Chrome optimisée
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	defer ctxCancel()

	info := &ScrapedCardInfo{}

	// Première tentative sans charger plus de contenu
	result := a.launchLoop(req.Quality, req.Language, req.Edition, false, ctx, url)
	
	// Si pas trouvé, essayer avec le chargement de plus de contenu
	if result == nil {
		log.Println("🔄 Première tentative échouée, essai avec chargement supplémentaire...")
		result = a.launchLoop(req.Quality, req.Language, req.Edition, true, ctx, url)
	}

	if result == nil {
		return nil, fmt.Errorf("aucune carte correspondant aux critères qualité=%s, langue=%s, édition=%t", req.Quality, req.Language, req.Edition)
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

	// Définir les autres champs avec des valeurs par défaut
	info.Set = "Set inconnu"
	info.Rarity = "Rareté inconnue"
	info.Offers = []CardOffer{*result}

	// Utiliser la carte trouvée
	info.Price = result.Price
	info.PriceNum = result.PriceNum
	log.Printf("✅ Offre sélectionnée: %s (mint: %s, langue: %s, edition: %t)",
		result.Price, result.Mint, result.Language, result.Edition)

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
	// Extraire le nombre du texte du prix (ex: "3,50 €" -> 3.50)
	re := regexp.MustCompile(`(\d+(?:[,.]\d+)?)`)
	matches := re.FindStringSubmatch(priceText)
	if len(matches) > 1 {
		priceStr := strings.Replace(matches[1], ",", ".", 1)
		if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
			return price
		}
	}
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
		if v, ok := cardData["edition"].(bool); ok {
			edition = v
		}

		cardOffer := CardOffer{
			Mint:     mint,
			Language: langue,
			Edition:  edition,
			Price:    price,
			PriceNum: a.extractNumericPrice(price),
		}

		log.Printf("Carte %d extraite: mint='%s', langue='%s', edition=%t, price='%s'\n", 
			i+1, cardOffer.Mint, cardOffer.Language, cardOffer.Edition, cardOffer.Price)

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

