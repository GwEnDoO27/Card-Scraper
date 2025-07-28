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

// Attendre que Cloudflare finisse
func (a *App) waitForCloudflare(ctx context.Context) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// Attendre jusqu'à 20 secondes que le titre ne contienne plus "Just a moment"
		for i := 0; i < 20; i++ {
			var title string
			err := chromedp.Title(&title).Do(ctx)
			if err != nil {
				return err
			}
			if !strings.Contains(title, "Just a moment") {
				log.Println("Protection Cloudflare contournée")
				return nil
			}
			time.Sleep(1 * time.Second)
		}
		log.Println("Timeout en attendant Cloudflare")
		return nil
	})
}

// Fermer la bannière de cookies
func (a *App) closeCookieBanner(ctx context.Context) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		time.Sleep(2 * time.Second)
		
		// Essayer plusieurs sélecteurs pour fermer les cookies
		cookieSelectors := []string{
			"#denyAll",
			"//button[contains(text(), 'Refuser')]",
			"//button[contains(text(), 'Reject')]",
			".cookie-banner button",
		}
		
		for _, selector := range cookieSelectors {
			var nodes []*cdp.Node
			var err error
			
			if strings.HasPrefix(selector, "//") {
				err = chromedp.Nodes(selector, &nodes, chromedp.BySearch).Do(ctx)
			} else {
				err = chromedp.Nodes(selector, &nodes, chromedp.ByQuery).Do(ctx)
			}
			
			if err == nil && len(nodes) > 0 {
				if strings.HasPrefix(selector, "//") {
					err = chromedp.Click(selector, chromedp.BySearch).Do(ctx)
				} else {
					err = chromedp.Click(selector, chromedp.ByQuery).Do(ctx)
				}
				if err == nil {
					log.Println("Bannière cookies fermée")
					return nil
				}
			}
		}
		
		log.Println("Pas de bannière cookies trouvée")
		return nil
	})
}

// Extraire toutes les offres de la page
func (a *App) extractOffers(ctx context.Context) ([]CardOffer, error) {
	var offers []CardOffer
	
	// Attendre et chercher toutes les lignes d'articles
	var articleNodes []*cdp.Node
	
	// Essayer de trouver les articles avec un délai d'attente
	for attempt := 0; attempt < 3; attempt++ {
		err := chromedp.Nodes(".article-row", &articleNodes, chromedp.ByQueryAll).Do(ctx)
		if err == nil && len(articleNodes) > 0 {
			break
		}
		log.Printf("Tentative %d: %d articles trouvés, nouvelle tentative...", attempt+1, len(articleNodes))
		time.Sleep(3 * time.Second)
	}
	
	if len(articleNodes) == 0 {
		return nil, fmt.Errorf("aucun article trouvé après 3 tentatives")
	}
	
	log.Printf("Nombre de lignes trouvées: %d", len(articleNodes))
	
	for i, node := range articleNodes {
		offer := CardOffer{}
		
		// Extraire la condition (mint)
		var mint string
		if err := chromedp.TextContent(".product-attributes .badge", &mint, chromedp.ByQuery, chromedp.FromNode(node)).Do(ctx); err == nil {
			offer.Mint = strings.TrimSpace(mint)
		}
		
		// Extraire la langue
		var langTitle string
		if err := chromedp.AttributeValue(".product-attributes .icon", "data-original-title", &langTitle, nil, chromedp.ByQuery, chromedp.FromNode(node)).Do(ctx); err == nil {
			offer.Language = strings.TrimSpace(langTitle)
		}
		
		// Vérifier si c'est une édition spéciale
		var editionNodes []*cdp.Node
		if err := chromedp.Nodes(".product-attributes .st_SpecialIcon", &editionNodes, chromedp.ByQueryAll, chromedp.FromNode(node)).Do(ctx); err == nil {
			offer.Edition = len(editionNodes) > 0
		}
		
		// Extraire le prix
		var priceText string
		if err := chromedp.TextContent(".price-container", &priceText, chromedp.ByQuery, chromedp.FromNode(node)).Do(ctx); err == nil {
			offer.Price = strings.TrimSpace(priceText)
			offer.PriceNum = a.extractNumericPrice(offer.Price)
		}
		
		if i < 3 { // Debug pour les 3 premières cartes
			log.Printf("Carte %d: %+v", i+1, offer)
		}
		
		offers = append(offers, offer)
	}
	
	return offers, nil
}

// Trouver la meilleure offre selon les critères
func (a *App) findBestOffer(offers []CardOffer, quality, language string, edition bool) *CardOffer {
	log.Printf("Recherche: mint='%s', langue='%s', edition=%t", quality, language, edition)
	log.Printf("Nombre total de cartes à examiner: %d", len(offers))
	
	for i, offer := range offers {
		log.Printf("Carte %d: mint='%s', langue='%s', edition=%t", i+1, offer.Mint, offer.Language, offer.Edition)
		if offer.Mint == quality && offer.Language == language && offer.Edition == edition {
			log.Printf("Carte trouvée: %+v", offer)
			return &offer
		}
	}
	
	log.Println("Carte non trouvée avec les critères exacts")
	return nil
}