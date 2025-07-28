package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

// Extraire TOUTES les offres de la page avec approche progressive
func (a *App) extractAllOffers(ctx context.Context) ([]CardOffer, error) {
	log.Println("=== EXTRACTION DES OFFRES (Approche progressive) ===")
	
	// Étape 1: Essayer de scraper la première page rapidement
	log.Println("🔍 ÉTAPE 1: Tentative scraping première page")
	offers, err := a.extractOffersFromCurrentPage(ctx)
	
	if err == nil && len(offers) > 0 {
		log.Printf("✅ SUCCÈS première page: %d offres trouvées", len(offers))
		return offers, nil
	}
	
	log.Printf("⚠️ Première page insuffisante (erreur: %v, offres: %d)", err, len(offers))
	log.Println("🔄 ÉTAPE 2: Scroll et clic 'Montrer plus' dans la même session")
	
	// Étape 2: Scroller et cliquer sur le bouton dans la session actuelle
	moreOffers, err := a.scrollAndLoadMore(ctx)
	if err == nil && len(moreOffers) > 0 {
		log.Printf("✅ SUCCÈS après clic bouton: %d offres trouvées", len(moreOffers))
		return moreOffers, nil
	}
	
	log.Printf("⚠️ Clic bouton échoué (erreur: %v, offres: %d)", err, len(moreOffers))
	log.Println("🔄 ÉTAPE 3: Nouvelle session complète en dernier recours")
	
	// Étape 3: En dernier recours, créer une nouvelle session
	var currentURL string
	chromedp.Location(&currentURL).Do(ctx)
	
	return a.extractOffersWithNewSession(currentURL)
}

// Extraire les offres de la page actuelle (tentative rapide)
func (a *App) extractOffersFromCurrentPage(ctx context.Context) ([]CardOffer, error) {
	log.Println("🔍 Scraping rapide de la page actuelle...")
	
	// Timeout court pour cette tentative
	quickCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	// Chercher immédiatement les articles
	var articleNodes []*cdp.Node
	err := chromedp.Nodes(".article-row", &articleNodes, chromedp.ByQueryAll).Do(quickCtx)
	if err != nil {
		return nil, fmt.Errorf("erreur recherche articles: %v", err)
	}
	
	if len(articleNodes) == 0 {
		return nil, fmt.Errorf("aucun article trouvé sur la page actuelle")
	}
	
	log.Printf("✅ %d articles trouvés, extraction rapide...", len(articleNodes))
	
	var offers []CardOffer
	// Limiter à 5 articles pour extraction rapide
	maxArticles := len(articleNodes)
	if maxArticles > 5 {
		maxArticles = 5
	}
	
	for i := 0; i < maxArticles; i++ {
		offer, err := a.extractOfferFromNode(quickCtx, articleNodes[i])
		if err != nil {
			log.Printf("⚠️ Article %d ignoré: %v", i+1, err)
			continue
		}
		offers = append(offers, *offer)
	}
	
	return offers, nil
}

// Scroller et cliquer sur "Montrer plus" dans la session actuelle
func (a *App) scrollAndLoadMore(ctx context.Context) ([]CardOffer, error) {
	log.Println("📜 Scroll et clic 'Montrer plus' dans la session actuelle...")
	
	// Timeout pour cette opération
	scrollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	// Étape 1: Scroller pour trouver le bouton
	log.Println("📜 Scroll vers le bas pour trouver le bouton...")
	for i := 0; i < 5; i++ {
		err := chromedp.Evaluate(`window.scrollBy(0, 1000);`, nil).Do(scrollCtx)
		if err != nil {
			log.Printf("Erreur scroll %d: %v", i+1, err)
		}
		time.Sleep(1 * time.Second)
	}
	
	// Étape 2: Chercher et cliquer sur le bouton
	log.Println("🔘 Recherche du bouton 'Montrer plus'...")
	
	loadMoreSelectors := []string{
		"#loadMoreButton",
		"//button[contains(text(), 'Montrer plus de résultats')]",
		"//button[contains(@class, 'btn-primary') and contains(text(), 'Montrer plus')]",
		".btn.btn-primary.btn-sm",
		"//button[contains(text(), 'Afficher plus')]",
		"//button[contains(text(), 'Charger plus')]",
	}
	
	clicked := false
	for _, selector := range loadMoreSelectors {
		var nodes []*cdp.Node
		var err error
		
		if strings.HasPrefix(selector, "//") {
			err = chromedp.Nodes(selector, &nodes, chromedp.BySearch).Do(scrollCtx)
		} else {
			err = chromedp.Nodes(selector, &nodes, chromedp.ByQueryAll).Do(scrollCtx)
		}
		
		if err == nil && len(nodes) > 0 {
			log.Printf("✅ Bouton trouvé avec sélecteur: %s", selector)
			
			// Essayer de cliquer
			var clickErr error
			if strings.HasPrefix(selector, "//") {
				clickErr = chromedp.Click(selector, chromedp.BySearch).Do(scrollCtx)
			} else {
				clickErr = chromedp.Click(selector, chromedp.ByQuery).Do(scrollCtx)
			}
			
			if clickErr == nil {
				log.Println("✅ Bouton cliqué avec succès!")
				clicked = true
				break
			} else {
				log.Printf("❌ Erreur clic: %v", clickErr)
			}
		}
	}
	
	if !clicked {
		return nil, fmt.Errorf("impossible de trouver ou cliquer sur le bouton 'Montrer plus'")
	}
	
	// Étape 3: Attendre que les nouveaux résultats se chargent
	log.Println("⏳ Attente du chargement des nouveaux résultats...")
	time.Sleep(5 * time.Second)
	
	// Étape 4: Extraire toutes les offres après le clic
	log.Println("🔍 Extraction des offres après le clic...")
	var articleNodes []*cdp.Node
	err := chromedp.Nodes(".article-row", &articleNodes, chromedp.ByQueryAll).Do(scrollCtx)
	if err != nil {
		return nil, fmt.Errorf("erreur recherche articles après clic: %v", err)
	}
	
	log.Printf("📊 %d articles trouvés après clic", len(articleNodes))
	
	if len(articleNodes) == 0 {
		return nil, fmt.Errorf("aucun article trouvé après le clic")
	}
	
	var offers []CardOffer
	// Limiter à 20 articles pour éviter les timeouts
	maxArticles := len(articleNodes)
	if maxArticles > 20 {
		maxArticles = 20
		log.Printf("⚠️ Limitation à %d articles", maxArticles)
	}
	
	for i := 0; i < maxArticles; i++ {
		offer, err := a.extractOfferFromNode(scrollCtx, articleNodes[i])
		if err != nil {
			log.Printf("⚠️ Article %d ignoré: %v", i+1, err)
			continue
		}
		offers = append(offers, *offer)
	}
	
	log.Printf("✅ %d offres extraites après scroll et clic", len(offers))
	return offers, nil
}

// Créer une nouvelle session et extraire avec le bouton "Montrer plus"
func (a *App) extractOffersWithNewSession(url string) ([]CardOffer, error) {
	log.Println("🆕 Création d'une nouvelle session Chrome...")
	
	// Créer une nouvelle session Chrome
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("exclude-switches", "enable-automation"),
		chromedp.Flag("disable-extensions", true),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel1 := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel1()

	newCtx, cancel2 := chromedp.NewContext(allocCtx)
	defer cancel2()

	newCtx, cancel3 := context.WithTimeout(newCtx, 60*time.Second)
	defer cancel3()
	
	var offers []CardOffer
	
	err := chromedp.Run(newCtx,
		// Naviguer vers la page
		chromedp.Navigate(url),
		chromedp.Evaluate(`Object.defineProperty(navigator, 'webdriver', {get: () => undefined})`, nil),
		
		// Attendre que la page charge
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("⏳ Attente chargement page...")
			time.Sleep(3 * time.Second)
			return nil
		}),
		
		// Scroller pour trouver le bouton
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("📜 Scroll pour trouver le bouton 'Montrer plus'...")
			
			// Scroller plusieurs fois vers le bas
			for i := 0; i < 3; i++ {
				err := chromedp.Evaluate(`window.scrollBy(0, 1000);`, nil).Do(ctx)
				if err != nil {
					log.Printf("Erreur scroll %d: %v", i+1, err)
				}
				time.Sleep(1 * time.Second)
			}
			
			return nil
		}),
		
		// Cliquer sur le bouton "Montrer plus"
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("🔘 Recherche et clic sur le bouton 'Montrer plus'...")
			
			loadMoreSelectors := []string{
				"#loadMoreButton",
				"//button[contains(text(), 'Montrer plus de résultats')]",
				"//button[contains(@class, 'btn-primary') and contains(text(), 'Montrer plus')]",
				".btn.btn-primary.btn-sm",
			}
			
			for _, selector := range loadMoreSelectors {
				var nodes []*cdp.Node
				var err error
				
				if strings.HasPrefix(selector, "//") {
					err = chromedp.Nodes(selector, &nodes, chromedp.BySearch).Do(ctx)
				} else {
					err = chromedp.Nodes(selector, &nodes, chromedp.ByQueryAll).Do(ctx)
				}
				
				if err == nil && len(nodes) > 0 {
					log.Printf("✅ Bouton trouvé avec sélecteur: %s", selector)
					
					// Essayer de cliquer
					var clickErr error
					if strings.HasPrefix(selector, "//") {
						clickErr = chromedp.Click(selector, chromedp.BySearch).Do(ctx)
					} else {
						clickErr = chromedp.Click(selector, chromedp.ByQuery).Do(ctx)
					}
					
					if clickErr == nil {
						log.Println("✅ Bouton cliqué avec succès!")
						time.Sleep(5 * time.Second) // Attendre que plus de résultats se chargent
						return nil
					} else {
						log.Printf("❌ Erreur clic: %v", clickErr)
					}
				}
			}
			
			log.Println("⚠️ Bouton non trouvé, continuation sans clic...")
			return nil
		}),
		
		// Extraire toutes les offres après le clic
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("🔍 Extraction finale des offres...")
			
			var articleNodes []*cdp.Node
			err := chromedp.Nodes(".article-row", &articleNodes, chromedp.ByQueryAll).Do(ctx)
			if err != nil {
				return fmt.Errorf("erreur recherche articles finaux: %v", err)
			}
			
			log.Printf("📊 %d articles trouvés après chargement", len(articleNodes))
			
			// Limiter à 15 articles pour éviter les timeouts
			maxArticles := len(articleNodes)
			if maxArticles > 15 {
				maxArticles = 15
				log.Printf("⚠️ Limitation à %d articles", maxArticles)
			}
			
			for i := 0; i < maxArticles; i++ {
				offer, err := a.extractOfferFromNode(ctx, articleNodes[i])
				if err != nil {
					log.Printf("⚠️ Article %d ignoré: %v", i+1, err)
					continue
				}
				offers = append(offers, *offer)
			}
			
			log.Printf("✅ %d offres extraites avec succès", len(offers))
			return nil
		}),
	)
	
	if err != nil {
		return nil, fmt.Errorf("erreur nouvelle session: %v", err)
	}
	
	return offers, nil
}

// Extraire une offre depuis un nœud article (fonction utilitaire)
func (a *App) extractOfferFromNode(ctx context.Context, node *cdp.Node) (*CardOffer, error) {
	offer := &CardOffer{}
	
	// Timeout court pour chaque extraction
	extractCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	
	// Mint
	var mint string
	err := chromedp.TextContent(".product-attributes .badge", &mint, chromedp.ByQuery, chromedp.FromNode(node)).Do(extractCtx)
	if err != nil {
		return nil, fmt.Errorf("mint non trouvé: %v", err)
	}
	offer.Mint = strings.TrimSpace(mint)
	
	// Langue
	var langue string
	err = chromedp.AttributeValue(".product-attributes .icon", "data-original-title", &langue, nil, chromedp.ByQuery, chromedp.FromNode(node)).Do(extractCtx)
	if err != nil {
		return nil, fmt.Errorf("langue non trouvée: %v", err)
	}
	offer.Language = strings.TrimSpace(langue)
	
	// Édition spéciale
	var editionNodes []*cdp.Node
	err = chromedp.Nodes(".product-attributes .st_SpecialIcon", &editionNodes, chromedp.ByQueryAll, chromedp.FromNode(node)).Do(extractCtx)
	offer.Edition = (err == nil && len(editionNodes) > 0)
	
	// Prix
	var price string
	err = chromedp.TextContent(".price-container", &price, chromedp.ByQuery, chromedp.FromNode(node)).Do(extractCtx)
	if err != nil {
		return nil, fmt.Errorf("prix non trouvé: %v", err)
	}
	offer.Price = strings.TrimSpace(price)
	offer.PriceNum = a.extractNumericPrice(offer.Price)
	
	return offer, nil
}

// Chercher la meilleure offre avec fallback (comme le script Python)
func (a *App) findBestOfferWithFallback(offers []CardOffer) *CardOffer {
	log.Printf("Recherche de la meilleure offre parmi %d offres", len(offers))
	
	// 1. Essayer d'abord: NM + Français + pas d'édition spéciale
	best := a.findBestOffer(offers, "NM", "Français", false)
	if best != nil {
		log.Println("Offre trouvée: NM + Français + standard")
		return best
	}
	
	// 2. Fallback: NM + Français + avec édition spéciale
	best = a.findBestOffer(offers, "NM", "Français", true)
	if best != nil {
		log.Println("Offre trouvée: NM + Français + édition spéciale")
		return best
	}
	
	// 3. Fallback: NM + n'importe quelle langue
	for _, offer := range offers {
		if offer.Mint == "NM" {
			log.Printf("Offre trouvée: NM + %s", offer.Language)
			return &offer
		}
	}
	
	// 4. Fallback: Première offre disponible
	if len(offers) > 0 {
		log.Printf("Fallback: première offre disponible (%s)", offers[0].Mint)
		return &offers[0]
	}
	
	log.Println("Aucune offre disponible")
	return nil
}