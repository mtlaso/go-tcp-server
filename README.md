But:

Permettre à plusieurs clients de communiquer ensemble.
Un client va envoyer un message à un serveur, qui va le relayer aux autres clients.
Avec un serveur TCP ?

tester : `nc -q 1 localhost 8080`

REQUIREMENTS
- [x] Broadcast messages à tous les clients (messages et messages serveur ex qqun quitte).
- [x] Sauvegarder messages (prendre un flag genre '--save-messages')
 - https://12factor.net/logs
 - `go run -race main.go -max-connected-clients=2 >> mylogs.log`
 - Pouvoir voir les messages en temps-reel (avec tail --follow)
- [x] limite connection
    - [x] afficher un message si plus de 5 conn, mettre dans un pool (queue ?)
    - [x] utiliser un param (--max-concurrent-clients-at-a-time)
- [x] max num of char (200 ?)
- [x] ajouter un/des jeux ?? (guess small word?)
- [x] flag.StringVar(&listenAddr, "listen-addr", "localhost:8080", "server listen address")
- [ ] ajouter des tests ?
- [x] enlever \n\n 2 fois en envoyant messages
