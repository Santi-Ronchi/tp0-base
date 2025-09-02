import socket
import logging
import signal
from common.utils import Bet, store_bets
from common.protocol import (
    deserialize_batch,
    read_message_from_socket,
    send_message_to_socket
)

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
            while True:  # Mantener la conexión abierta para múltiples batches
                try:
                    # Read complete message using protocol module
                    msg = read_message_from_socket(client_sock)
                    
                    # Si no hay mensaje, la conexión se cerró
                    if not msg:
                        break
                    
                    # Parse batch message using protocol module
                    bets_data = deserialize_batch(msg)
                    
                    # Si el batch está vacío, responder OK pero no procesar
                    if len(bets_data) == 0:
                        send_message_to_socket(client_sock, "OK")
                        logging.info(f"action: apuesta_recibida | result: success | cantidad: 0")
                        continue
                    
                    # Create Bet objects
                    bets = []
                    for bet_data in bets_data:
                        bet = Bet(
                            agency=bet_data["agency"],
                            first_name=bet_data["first_name"],
                            last_name=bet_data["last_name"],
                            document=bet_data["document"],
                            birthdate=bet_data["birthdate"],
                            number=bet_data["number"]
                        )
                        bets.append(bet)
                    
                    # Store all bets in the batch
                    store_bets(bets)
                    
                    # Send confirmation using protocol module
                    send_message_to_socket(client_sock, "OK")
                    
                    # Log success with batch size
                    logging.info(f"action: apuesta_recibida | result: success | cantidad: {len(bets)}")
                    
                except ConnectionError:
                    # Cliente cerró la conexión, es normal
                    break
                except Exception as e:
                    # Error procesando el batch actual
                    logging.error(f"action: apuesta_recibida | result: fail | error: {e}")
                    try:
                        # Try to send error response
                        send_message_to_socket(client_sock, "ERROR")
                        # Log failure - intentamos extraer la cantidad si es posible
                        try:
                            msg_len = len(msg.split(';')) if 'msg' in locals() else 0
                            logging.error(f"action: apuesta_recibida | result: fail | cantidad: {msg_len}")
                        except:
                            logging.error(f"action: apuesta_recibida | result: fail | cantidad: unknown")
                    except:
                        pass  # Ignore errors when sending error response
                    break  # Salir del loop en caso de error
                    
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