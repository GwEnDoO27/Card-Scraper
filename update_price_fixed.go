package main

import (
	"time"
)

// Mettre à jour le prix d'une carte (fonction corrigée)
func (a *App) UpdateCardPriceFixed(cardID int) (*Card, error) {
	// Récupérer la carte
	card, err := a.getCardByID(cardID)
	if err != nil {
		return nil, err
	}

	// Scraper le nouveau prix avec critères par défaut
	defaultReq := AddCardRequest{
		Quality:  "NM",
		Language: "Français",
		Edition:  false,
	}
	cardInfo, err := a.scrapeCardInfo(card.CardURL, defaultReq)
	if err != nil {
		return nil, err
	}

	// Mettre à jour en base
	_, err = a.db.Exec(`
		UPDATE cards 
		SET price = ?, price_num = ?, last_updated = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, cardInfo.Price, cardInfo.PriceNum, cardID)
	if err != nil {
		return nil, err
	}

	card.Price = cardInfo.Price
	card.PriceNum = cardInfo.PriceNum
	card.LastUpdated = time.Now().Format("2006-01-02 15:04:05")

	return card, nil
}
