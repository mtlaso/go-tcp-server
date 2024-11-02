But:

Permettre à plusieurs clients de communiquer ensemble.
Un client va envoyer un message à un serveur, qui va le relayer aux autres clients.
Avec un serveur TCP ?

tester : `nc -q 1 localhost 8080`

REQUIREMENTS
- [ ] Broadcast messages à tous les clients (messages et messages serveur ex qqun quitte).
- [ ] Sauvegarder messages (prendre un flag genre '--save-messages')
- [ ] Pouvoir voir les messages en temps-reel (avec tail --follow)
- [ ] limite connection
    - [ ] afficher un message si plus de 5 conn, mettre dans un pool (queue ?)
- [ ] Faire un test pour savoir le nb max de connections que le serveur peut supporter
- [ ] traductions des messages du serveur avec commande
- [ ] traductions des messages des clients en temps réel (avec commande spéciale)