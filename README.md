<preview-gif>

[*english*](#En)

# Serveur de Chat TCP en Go
<preview-gif>
Un serveur de chat TCP concurrent développé en Go qui permet la communication en temps réel, la gestion des connexions et un jeu interactif.

## 🎯 Fonctionnalités
- **Chat en Temps Réel**
  - Chat entre plusieurs usagers
  - Broadcasting des messages à tous les usagers connectés
  - Notifications du serveur pour les connexions/déconnexions

- **Gestion des Connexions**
  - Limite de connexions configurable
  - Système de file d'attente pour les connexions excédentaires
  - Notifications de position dans la file

- **Système de Jeu**
  - Jeu de devinettes de mots intégré
  - Sessions de jeu simultanées
  - Broadcasting en temps réel de l'état du jeu

- **Gestion des Erreurs**
  - Fermeture contrôlée du serveur
  - Gestion des délais de connexion
  - Journalisation complète des erreurs
  - Prévention des conditions de concurrence

## 🚀 Pour Commencer
### Prérequis
- Go 1.21 ou plus récent
- netcat (nc) pour tester les connexions

### Installation
```bash
# Cloner le répertoire
git clone https://github.com/mtlaso/go-tcp-server
cd go-tcp-server
```

### Utilisation
Démarrer le serveur :
```bash
go run main.go --listen-addr=localhost:8080 # Option 1.
go run main.go --max-connected-clients=10 --listen-addr=localhost:8080 # Option 2.
go run -h # Voir les options
```

Se connecter comme client :
```bash
nc -q -1 localhost 8080
```

Activer la journalisation :
```bash
go run main.go --max-connected-clients=10 >> mylogs.log
tail -f mylogs.log  # Voir les journaux en temps réel
```

## 🎮 Commandes Disponibles
| Commande | Description |
|----------|-------------|
| `/count` | Afficher le nombre d'usagers connectés |
| `/commands` | Afficher les commandes disponibles |
| `/game` | Démarrer une partie de devinettes |
| `/endgame` | Terminer la partie en cours |

## 🛠️ Stack
- Go
- Packages de la bibliothèque standard (net, context, sync)

# En
# Go TCP Chat Server

<preview-gif>

A concurrent TCP chat server implemented in Go that demonstrates real-time communication, connection management, and interactive gameplay.
## 🎯 Features

- **Real-time Chat**
  - Multi-client messaging system
  - Broadcast messages to all connected clients
  - Server notifications for client connections/disconnections

- **Connection Management**
  - Configurable client connection limits
  - Smart queuing system for excess connections
  - Queue position notifications

- **Interactive Game System**
  - Built-in word guessing game
  - Concurrent game sessions
  - Real-time game state broadcasting

- **Robust Error Handling**
  - Graceful shutdown mechanism
  - Connection timeout handling
  - Comprehensive error logging
  - Race condition prevention

## 🚀 Getting Started

### Prerequisites
- Go 1.21 or higher
- netcat (nc) for client testing

### Installation
```bash
# Clone the repository
git clone https://github.com/mtlaso/go-tcp-server
cd go-tcp-server
```

### Usage

Start the server:
```bash

go run main.go --listen-addr=localhost:8080 # Option 1.
go run main.go --max-connected-clients=10 --listen-addr=localhost:8080 # Option 2.
go run -h # Show options
```

Connect as a client:
```bash
nc -q -1 localhost 8080
```

Enable logging:
```bash
go run main.go --max-connected-clients=10 >> mylogs.log
tail -f mylogs.log  # View logs in real-time
```

## 🎮 Available Commands

| Command | Description |
|---------|-------------|
| `/count` | Display number of connected clients |
| `/commands` | Show available commands |
| `/game` | Start a word guessing game |
| `/endgame` | End current game session |

## 🛠️ Tech Stack
- Go
- Standard library packages (net, context, sync)
