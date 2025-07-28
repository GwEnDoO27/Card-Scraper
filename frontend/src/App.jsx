import {useState, useEffect} from 'react';
import {AddCard, GetCards, DeleteCard, MoveCard} from '../wailsjs/go/main/App';

function App() {
    const [activeTab, setActiveTab] = useState('collection');
    const [collectionCards, setCollectionCards] = useState([]);
    const [wishlistCards, setWishlistCards] = useState([]);
    const [newCardUrl, setNewCardUrl] = useState('');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [darkMode, setDarkMode] = useState(() => {
        return localStorage.getItem('darkMode') === 'true';
    });
    const [searchCriteria, setSearchCriteria] = useState({
        quality: 'NM',
        language: 'Français',
        edition: false
    });

    // Gérer le mode sombre
    useEffect(() => {
        document.documentElement.setAttribute('data-theme', darkMode ? 'dark' : 'light');
        localStorage.setItem('darkMode', darkMode);
    }, [darkMode]);

    const toggleDarkMode = () => {
        setDarkMode(!darkMode);
    };

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
        <div className="app-bg font-['Nunito']">
            {/* Toggle thème minimaliste */}
            <div className="theme-toggle" onClick={toggleDarkMode}>
                {darkMode ? '☀' : '🌙'}
            </div>

            {/* Header minimaliste */}
            <header className="header-glass py-8">
                <div className="max-w-4xl mx-auto px-6 text-center">
                    <h1 className="text-3xl font-light mb-2" style={{color: 'var(--text-primary)'}}>
                        Card Collection
                    </h1>
                    <p className="text-sm" style={{color: 'var(--text-secondary)'}}>
                        Yu-Gi-Oh! Manager
                    </p>
                </div>
            </header>

            {/* Navigation minimaliste */}
            <nav className="nav-glass">
                <div className="max-w-4xl mx-auto px-6 py-4">
                    <div className="flex gap-1">
                        <button 
                            className={`btn-secondary px-6 py-2 text-sm ${
                                activeTab === 'collection' ? 'active' : ''
                            }`}
                            onClick={() => setActiveTab('collection')}
                        >
                            Collection
                        </button>
                        <button 
                            className={`btn-secondary px-6 py-2 text-sm ${
                                activeTab === 'wishlist' ? 'active' : ''
                            }`}
                            onClick={() => setActiveTab('wishlist')}
                        >
                            Wishlist
                        </button>
                    </div>
                </div>
            </nav>

            <main className="max-w-4xl mx-auto p-6">
                {error && (
                    <div className="mb-6 glass p-4 rounded-2xl" style={{
                        borderColor: '#ef4444',
                        background: 'rgba(239, 68, 68, 0.1)',
                        color: '#ef4444'
                    }}>
                        {error}
                    </div>
                )}
                
                <div className="glass-strong p-8 mb-8 rounded-3xl">
                    <h2 className="text-xl font-medium mb-6" style={{color: 'var(--text-primary)'}}>
                        Add New Card
                    </h2>
                    
                    {/* URL Input */}
                    <div className="mb-6">
                        <label className="block text-sm mb-3" style={{color: 'var(--text-secondary)'}}>
                            Card URL
                        </label>
                        <input
                            type="url"
                            placeholder="Enter CardMarket URL..."
                            value={newCardUrl}
                            onChange={(e) => setNewCardUrl(e.target.value)}
                            className="w-full input-glass px-4 py-3"
                            disabled={loading}
                        />
                    </div>

                    {/* Search Criteria */}
                    <div className="mb-8">
                        <h3 className="text-sm mb-4" style={{color: 'var(--text-secondary)'}}>
                            Search Criteria
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                            {/* Quality */}
                            <div>
                                <label className="block text-xs mb-2" style={{color: 'var(--text-secondary)'}}>
                                    Quality
                                </label>
                                <select
                                    value={searchCriteria.quality}
                                    onChange={(e) => setSearchCriteria({...searchCriteria, quality: e.target.value})}
                                    className="w-full input-glass px-3 py-2 text-sm"
                                    disabled={loading}
                                >
                                    <option value="EX">Excellent</option>
                                    <option value="NM">Near Mint</option>
                                    <option value="GD">Good</option>
                                    <option value="LP">Lightly Played</option>
                                    <option value="PO">Poor</option>
                                </select>
                            </div>

                            {/* Language */}
                            <div>
                                <label className="block text-xs mb-2" style={{color: 'var(--text-secondary)'}}>
                                    Language
                                </label>
                                <select
                                    value={searchCriteria.language}
                                    onChange={(e) => setSearchCriteria({...searchCriteria, language: e.target.value})}
                                    className="w-full input-glass px-3 py-2 text-sm"
                                    disabled={loading}
                                >
                                    <option value="Français">Français</option>
                                    <option value="English">English</option>
                                    <option value="Deutsch">Deutsch</option>
                                    <option value="Español">Español</option>
                                    <option value="Italiano">Italiano</option>
                                    <option value="日本語">日本語</option>
                                </select>
                            </div>

                            {/* Edition */}
                            <div>
                                <label className="block text-xs mb-2" style={{color: 'var(--text-secondary)'}}>
                                    Edition
                                </label>
                                <div className="flex items-center space-x-4 pt-1">
                                    <label className="flex items-center cursor-pointer text-sm">
                                        <input
                                            type="radio"
                                            name="edition"
                                            checked={!searchCriteria.edition}
                                            onChange={() => setSearchCriteria({...searchCriteria, edition: false})}
                                            className="mr-2 text-blue-500 focus:ring-blue-400"
                                            disabled={loading}
                                        />
                                        <span style={{color: 'var(--text-primary)'}}>Standard</span>
                                    </label>
                                    <label className="flex items-center cursor-pointer text-sm">
                                        <input
                                            type="radio"
                                            name="edition"
                                            checked={searchCriteria.edition}
                                            onChange={() => setSearchCriteria({...searchCriteria, edition: true})}
                                            className="mr-2 text-blue-500 focus:ring-blue-400"
                                            disabled={loading}
                                        />
                                        <span style={{color: 'var(--text-primary)'}}>First Edition</span>
                                    </label>
                                </div>
                            </div>
                        </div>
                    </div>

                    {/* Add Button */}
                    <button 
                        onClick={addCard} 
                        disabled={loading || !newCardUrl.trim()}
                        className={`w-full btn-primary px-6 py-3 font-medium disabled:opacity-50 disabled:cursor-not-allowed ${loading ? 'loading-minimal' : ''}`}
                    >
                        {loading ? 'Adding...' : 'Add Card'}
                    </button>
                </div>

                <div>
                    <div className="flex items-center justify-between mb-6">
                        <h3 className="text-lg font-medium" style={{color: 'var(--text-primary)'}}>
                            {currentCards.length} {currentCards.length === 1 ? 'Card' : 'Cards'}
                        </h3>
                    </div>
                    
                    {currentCards.length === 0 ? (
                        <div className="text-center py-20 glass rounded-3xl">
                            <p className="text-lg mb-2" style={{color: 'var(--text-secondary)'}}>
                                No cards yet
                            </p>
                            <p className="text-sm" style={{color: 'var(--text-secondary)'}}>
                                Add your first card using the form above
                            </p>
                        </div>
                    ) : (
                        <div className="space-y-4">
                            {currentCards.map(card => (
                                <div key={card.id} className="card-glass p-6 group">
                                    <div className="flex items-start gap-6">
                                        {card.image_url && (
                                            <img 
                                                src={card.image_url} 
                                                alt={card.name}
                                                className="w-16 h-20 object-cover rounded-xl flex-shrink-0"
                                            />
                                        )}
                                        <div className="flex-1 min-w-0">
                                            <h4 className="text-lg font-medium mb-2" style={{color: 'var(--text-primary)'}}>
                                                {card.name || 'Unknown Card'}
                                            </h4>
                                            
                                            {/* Card details */}
                                            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-3 text-sm">
                                                <div>
                                                    <span style={{color: 'var(--text-secondary)'}}>Set</span>
                                                    <div style={{color: 'var(--text-primary)'}}>{card.set_name || 'Unknown'}</div>
                                                </div>
                                                <div>
                                                    <span style={{color: 'var(--text-secondary)'}}>Rarity</span>
                                                    <div style={{color: 'var(--text-primary)'}}>{card.rarity || 'Unknown'}</div>
                                                </div>
                                                {card.quality && (
                                                    <div>
                                                        <span style={{color: 'var(--text-secondary)'}}>Quality</span>
                                                        <div className="inline-block px-2 py-1 rounded-lg text-xs font-medium" style={{
                                                            background: 'var(--accent)',
                                                            color: 'white'
                                                        }}>{card.quality}</div>
                                                    </div>
                                                )}
                                                {card.language && (
                                                    <div>
                                                        <span style={{color: 'var(--text-secondary)'}}>Language</span>
                                                        <div style={{color: 'var(--text-primary)'}}>{card.language}</div>
                                                    </div>
                                                )}
                                            </div>
                                            
                                            {/* Card link */}
                                            <a 
                                                href={card.card_url} 
                                                target="_blank" 
                                                rel="noopener noreferrer" 
                                                className="text-sm hover:underline transition-colors"
                                                style={{color: 'var(--accent)'}}
                                            >
                                                View on CardMarket →
                                            </a>
                                        </div>
                                        
                                        {/* Price and actions */}
                                        <div className="flex flex-col items-end gap-4">
                                            <div className="text-right">
                                                <div className="text-2xl font-semibold" style={{color: 'var(--accent)'}}>
                                                    {card.price || 'N/A'}
                                                </div>
                                                <div className="text-xs" style={{color: 'var(--text-secondary)'}}>
                                                    {new Date(card.added_at).toLocaleDateString()}
                                                </div>
                                            </div>
                                            
                                            {/* Action buttons */}
                                            <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                                {activeTab === 'wishlist' && (
                                                    <button 
                                                        onClick={() => moveCard(card.id, 'collection')}
                                                        className="btn-secondary px-3 py-1 text-xs"
                                                    >
                                                        Move to Collection
                                                    </button>
                                                )}
                                                <button 
                                                    onClick={() => removeCard(card.id)}
                                                    className="w-8 h-8 rounded-lg flex items-center justify-center transition-all hover:scale-110"
                                                    style={{
                                                        background: 'rgba(239, 68, 68, 0.1)',
                                                        color: '#ef4444',
                                                        border: '1px solid rgba(239, 68, 68, 0.2)'
                                                    }}
                                                >
                                                    ×
                                                </button>
                                            </div>
                                        </div>
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