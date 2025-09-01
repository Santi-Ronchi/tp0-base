import socket
import logging
import signal
import json
from common.utils import *


class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self.running = True
        signal.signal(signal.SIGTERM, self.graceful_shutdown)

    def graceful_shutdown(self, signum, frame):
        logging.info("SIGTERM received, shutting down server gracefully")
        self.running = False
        if self._server_socket:
            self._server_socket.close()
            logging.info("Server socket closed")

    def run(self):
        """
        Server loop that accepts new connections and establishes
        communication with clients (agencies). After client communication
        finishes, server starts to accept new connections again
        """
        while self.running:
            try:
                client_sock = self.__accept_new_connection()
                if client_sock:
                    self.__handle_client_connection(client_sock)
            except OSError:
                # Socket principal cerrado
                if not self.running:
                    break
                else:
                    logging.error("Unexpected OSError while accepting connections")
        logging.info("Server loop exited gracefully")

    def __handle_client_connection(self, client_sock):
        """
        Read message from a specific client socket and closes the socket
        If a problem arises in the communication with the client, the
        client socket will also be closed
        """
        try:
            # Read complete message until newline (avoid short-read)
            data = b""
            while True:
                chunk = client_sock.recv(1024)
                if not chunk:
                    if not data:
                        logging.warning("Client disconnected without sending data")
                        return
                    break
                data += chunk
                if b"\n" in data:
                    # Found complete message
                    break
            
             # Process only the first message (up to newline)
            messages = data.split(b"\n")
            if not messages[0]:
                logging.warning("Empty message received")
                return
                
            msg = messages[0].decode("utf-8").strip()
            
            # Parse JSON bet data
            bet_json = json.loads(msg)
            
            # Create Bet object
            bet = Bet(
                agency=bet_json["agency"],
                first_name=bet_json["first_name"],
                last_name=bet_json["last_name"],
                document=bet_json["document"],
                birthdate=bet_json["birthdate"],
                number=bet_json["number"]
            )

            # Guardar la apuesta
            store_bets([bet])

            # Confirmación
            confirmation = b"OK\n"
            total_sent = 0
            while total_sent < len(confirmation):
                sent = client_sock.send(confirmation[total_sent:])
                if sent == 0:
                    raise RuntimeError("Socket connection broken")
                total_sent += sent
                client_sock.sendall(b"OK\n")
                logging.info(f"action: apuesta_almacenada | result: success | dni: {bet.document} | numero: {bet.number}")
        
        except Exception as e:
            logging.error(f"action: apuesta_almacenada | result: fail | error: {e}")
        finally:
            client_sock.close()
            logging.debug("Client socket closed")

    def __accept_new_connection(self):
        """
        Accept new connections
        Function blocks until a connection to a client is made.
        Then connection created is printed and returned
        """

        try:
            # Connection arrived
            logging.info('action: accept_connections | result: in_progress')
            c, addr = self._server_socket.accept()
            logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
            return c
        except OSError:
            if not self.running:
                # Expected when shutting down
                return None
            raise