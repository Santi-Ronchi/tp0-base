import socket
import logging
import signal
import json
from utils import *


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
        Dummy Server loop

        Server that accept a new connections and establishes a
        communication with a client. After client with communucation
        finishes, servers starts to accept new connections again
        """
        while self.running:
            try:
                client_sock = self.__accept_new_connection()
                self.__handle_client_connection(client_sock)
            except OSError:
                # Socket principal cerrado
                break
        logging.info("Server loop exited gracefully")

    def __handle_client_connection(self, client_sock):
        """
        Read message from a specific client socket and closes the socket

        If a problem arises in the communication with the client, the
        client socket will also be closed
        """
        try:
            data = b""
            while not data.endswith(b"\n"):
                chunk = client_sock.recv(1024)
                if not chunk:
                    break
                data += chunk

            msg = data.decode("utf-8").strip()
            bet_json = json.loads(msg)

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
            client_sock.sendall(b"OK\n")

            logging.info(f"action: apuesta_almacenada | result: success | dni: {bet.document} | numero: {bet.number}")

        except Exception as e:
            logging.error(f"action: apuesta_almacenada | result: fail | error: {e}")
        finally:
            client_sock.close()
            logging.info("Client socket closed")

    def __accept_new_connection(self):
        """
        Accept new connections

        Function blocks until a connection to a client is made.
        Then connection created is printed and returned
        """

        # Connection arrived
        logging.info('action: accept_connections | result: in_progress')
        c, addr = self._server_socket.accept()
        logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
        return c
