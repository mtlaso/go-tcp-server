But:

Permettre à plusieurs clients de communiquer ensemble.
Un client va envoyer un message à un serveur, qui va le relayer aux autres clients.
Avec un serveur TCP ?

tester : `nc localhost 8080`

REQUIREMENTS
- Sauvegarder messages
- Pouvoir voir les messages en temps-reel (tail --follow)
- limite connection
    - afficher un message si plus de 5 conn, mettre dans un pool (queue ?)
