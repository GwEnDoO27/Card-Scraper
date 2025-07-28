import {useState, useEffect} from 'react';
import {AddCard, GetCards, DeleteCard, MoveCard} from '../wailsjs/go/main/App';

function App() {
    const [activeTab, setActiveTab] = useState('collection');
    const [collectionCards, setCollectionCards] = useState([]);
    const [wishlistCards, setWishlistCards] = useState([]);
    const [newCardUrl, setNewCardUrl] = useState('');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [searchCriteria, setSearchCriteria] = useState({
        quality: 'NM',
        language: 'Français',
        edition: false
    });

    const addCard = async () => {
        if (!newCardUrl.trim()) return;
        
        setLoading(true);
        setError('');
        
        try {
            const card = await AddCard({
                url: newCardUrl,
                type: activeTab,
                quality: searchCriteria.quality,
                language: searchCriteria.language,
                edition: searchCriteria.edition
            });
            
            if (activeTab === 'collection') {
                setCollectionCards([card, ...collectionCards]);
            } else {
                setWishlistCards([card, ...wishlistCards]);
            }
            
            setNewCardUrl('');
        } catch (err) {
            setError(err.message || 'Erreur lors de l\'ajout de la carte');
        } finally {
            setLoading(false);
        }
    };

    const removeCard = async (id) => {
        try {
            await DeleteCard(id);
            
            if (activeTab === 'collection') {
                setCollectionCards(collectionCards.filter(card => card.id !== id));
            } else {
                setWishlistCards(wishlistCards.filter(card => card.id !== id));
            }
        } catch (err) {
            setError('Erreur lors de la suppression');
        }
    };
    
    const moveCard = async (cardId, newType) => {
        try {
            await MoveCard(cardId, newType);
            loadCards();
        } catch (err) {
            setError('Erreur lors du déplacement');
        }
    };
    
    const loadCards = async () => {
        try {
            const [collection, wishlist] = await Promise.all([
                GetCards('collection'),
                GetCards('wishlist')
            ]);
            setCollectionCards(collection || []);
            setWishlistCards(wishlist || []);
        } catch (err) {
            setError('Erreur lors du chargement des cartes');
        }
    };
    
    useEffect(() => {
        loadCards();
    }, []);

    const currentCards = activeTab === 'collection' ? collectionCards : wishlistCards;

    return (
        <div className="min-h-screen bg-slate-50 font-['Nunito']">
            <header className="bg-gradient-to-r from-indigo-500 to-purple-600 text-white py-8 text-center shadow-lg">
                <h1 className="text-4xl font-bold">Card Scraper</h1>
            </header>

            <nav className="bg-white border-b border-slate-200 px-8">
                <div className="flex gap-4">
                    <button 
                        className={`px-8 py-4 text-lg font-medium border-b-4 transition-all ${
                            activeTab === 'collection' 
                                ? 'text-indigo-600 border-indigo-600 bg-slate-50' 
                                : 'text-slate-500 border-transparent hover:text-slate-700 hover:bg-slate-50'
                        }`}
                        onClick={() => setActiveTab('collection')}
                    >
                        Collection
                    </button>
                    <button 
                        className={`px-8 py-4 text-lg font-medium border-b-4 transition-all ${
                            activeTab === 'wishlist' 
                                ? 'text-indigo-600 border-indigo-600 bg-slate-50' 
                                : 'text-slate-500 border-transparent hover:text-slate-700 hover:bg-slate-50'
                        }`}
                        onClick={() => setActiveTab('wishlist')}
                    >
                        Wishlist
                    </button>
                </div>
            </nav>

            <main className="max-w-6xl mx-auto p-8">
                {error && (
                    <div className="mb-6 p-4 bg-red-50 border border-red-200 text-red-700 rounded-lg">
                        {error}
                    </div>
                )}
                
                <div className="bg-white rounded-xl p-8 mb-8 shadow-sm">
                    <h2 className="text-2xl font-semibold mb-6 text-slate-800">
                        Ajouter une carte à {activeTab === 'collection' ? 'la collection' : 'la wishlist'}
                    </h2>
                    <div className="flex gap-4">
                        <input
                            type="url"
                            placeholder="Entrez l'URL de la carte..."
                            value={newCardUrl}
                            onChange={(e) => setNewCardUrl(e.target.value)}
                            className="flex-1 border-2 border-slate-200 rounded-lg px-4 py-3 text-lg focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 transition-colors"
                            disabled={loading}
                        />
                        <button 
                            onClick={addCard} 
                            disabled={loading || !newCardUrl.trim()}
                            className="bg-gradient-to-r from-indigo-500 to-purple-600 text-white px-8 py-3 rounded-lg text-lg font-semibold hover:shadow-lg hover:-translate-y-0.5 transition-all disabled:opacity-50 disabled:cursor-not-allowed disabled:transform-none"
                        >
                            {loading ? 'Ajout...' : 'Ajouter'}
                        </button>
                    </div>
                </div>

                <div>
                    <h3 className="text-xl font-semibold mb-6 text-slate-700">
                        {currentCards.length} carte{currentCards.length !== 1 ? 's' : ''}
                    </h3>
                    
                    {currentCards.length === 0 ? (
                        <div className="text-center py-16 bg-white rounded-lg border-2 border-dashed border-slate-200">
                            <p className="text-lg text-slate-500">
                                Aucune carte dans {activeTab === 'collection' ? 'la collection' : 'la wishlist'}
                            </p>
                        </div>
                    ) : (
                        <div className="grid gap-4">
                            {currentCards.map(card => (
                                <div key={card.id} className="bg-white border border-slate-200 rounded-lg p-6 flex items-center justify-between hover:shadow-md hover:-translate-y-0.5 transition-all group">
                                    <div className="flex items-center gap-6">
                                        {card.image_url && (
                                            <img 
                                                src={card.image_url} 
                                                alt={card.name}
                                                className="w-16 h-20 object-cover rounded border"
                                            />
                                        )}
                                        <div className="flex-1">
                                            <h4 className="text-lg font-semibold text-slate-800 mb-1">
                                                {card.name || 'Carte inconnue'}
                                            </h4>
                                            <p className="text-sm text-slate-500 mb-2">
                                                {card.set_name} • {card.rarity}
                                            </p>
                                            <a 
                                                href={card.card_url} 
                                                target="_blank" 
                                                rel="noopener noreferrer" 
                                                className="text-indigo-600 hover:text-indigo-800 text-sm font-medium hover:underline"
                                            >
                                                Voir la carte
                                            </a>
                                        </div>
                                        <div className="text-right">
                                            <div className="text-xl font-bold text-green-600 mb-1">
                                                {card.price || 'Prix N/A'}
                                            </div>
                                            <div className="text-xs text-slate-500">
                                                Ajouté le {new Date(card.added_at).toLocaleDateString()}
                                            </div>
                                        </div>
                                    </div>
                                    <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                        <button 
                                            onClick={() => moveCard(card.id, activeTab === 'collection' ? 'wishlist' : 'collection')}
                                            className="px-3 py-1 text-sm bg-blue-100 text-blue-600 rounded hover:bg-blue-200 transition-colors"
                                        >
                                            {activeTab === 'collection' ? '→ Wishlist' : '→ Collection'}
                                        </button>
                                        <button 
                                            onClick={() => removeCard(card.id)}
                                            className="w-8 h-8 bg-red-500 text-white rounded hover:bg-red-600 transition-colors flex items-center justify-center text-lg"
                                        >
                                            ×
                                        </button>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            </main>
        </div>
    )
}

export default App
