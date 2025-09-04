import socket
import logging
import signal
import threading
from common.utils import Bet, store_bets, load_bets, has_won
from common.protocol import (
    deserialize_batch,
    read_message_from_socket,
    send_message_to_socket
)

class Server:
    def __init__(self, port, listen_backlog, total_agencies):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self.running = True
        signal.signal(signal.SIGTERM, self.graceful_shutdown)
        
        # Concurrency control
        self.lottery_lock = threading.Lock()
        self.agencies_finished = set()  # Agencias que terminaron de enviar
        self.lottery_done = False  # Indica si se realizó el sorteo
        self.winners_by_agency = {}  # DNIs ganadores por agencia
        self.waiting_clients = []  # Clientes esperando resultados
        self.total_agencies = total_agencies  # Total de agencias esperadas

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
                    # manage each client connection in a separate thread
                    client_thread = threading.Thread(
                        target=self.__handle_client_connection,
                        args=(client_sock,)
                    )
                    client_thread.daemon = True
                    client_thread.start()
            except OSError:
                # main socket was closed, likely due to SIGTERM
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
        agency_id = None
        try:
            while True:  # maintain connection until client closes it
                try:
                    # Read complete message using protocol module
                    msg = read_message_from_socket(client_sock)
                    
                    if msg.startswith("BET:"):
                        # bet batch received
                        batch_data = msg[4:]  # Remover prefijo "BET:"
                        agency_id = self.__process_bets_batch(batch_data)
                        
                        # Send confirmation
                        send_message_to_socket(client_sock, "OK")
                        
                    elif msg == "FINISHED":
                        # Client notifies it finished sending bets
                        if agency_id is not None:
                            self.__handle_finished_notification(agency_id)
                        send_message_to_socket(client_sock, "OK")
                        
                    elif msg == "WINNERS":
                        # Client requests winners for its agency
                        winners = self.__handle_winners_query(agency_id, client_sock)
                        if winners is not None:
                            # Send winners immediately if available
                            send_message_to_socket(client_sock, winners)
                        # if winners is None, the client was added to waiting list
                        
                except ConnectionError as e:
                    logging.debug(f"Client closed connection: {e}")
                    break
                except Exception as e:
                    logging.error(f"action: message_processing | result: fail | error: {e}")
                    try:
                        send_message_to_socket(client_sock, "ERROR")
                    except:
                        pass
                    break
                    
        finally:
            # Clean up list of waiting clients if needed
            with self.lottery_lock:
                self.waiting_clients = [(sock, aid) for sock, aid in self.waiting_clients 
                                       if sock != client_sock]
            client_sock.close()
            logging.debug("Client socket closed")

    def __process_bets_batch(self, batch_data):
        """
        Procesa un batch de apuestas y retorna el ID de la agencia
        """
        # Parse batch message using protocol module
        bets_data = deserialize_batch(batch_data)
        
        # if no bets, log and return None
        if len(bets_data) == 0:
            logging.info(f"action: apuesta_recibida | result: success | cantidad: 0")
            return None
        
        agency_id = None
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
            if agency_id is None:
                agency_id = bet_data["agency"]
        
        # Store all bets in the batch
        store_bets(bets)
        
        # Log success with batch size
        logging.info(f"action: apuesta_recibida | result: success | cantidad: {len(bets)}")
        
        return agency_id

    def __handle_finished_notification(self, agency_id):
        """
        Maneja la notificación de que una agencia terminó de enviar apuestas
        """
        with self.lottery_lock:
            self.agencies_finished.add(str(agency_id))
            logging.info(f"Agency {agency_id} finished sending bets. Total finished: {len(self.agencies_finished)}/{self.total_agencies}")
            
            # Verify if all agencies have finished
            if len(self.agencies_finished) >= self.total_agencies and not self.lottery_done:
                self.__perform_lottery()

    def __perform_lottery(self):
        """
        Realiza el sorteo y determina los ganadores por agencia
        Debe ser llamado con lottery_lock adquirido
        """
        logging.info("action: sorteo | result: success")
        self.lottery_done = True
        
        # load all bets
        all_bets = list(load_bets())
        
        # group winners by agency
        self.winners_by_agency = {}
        for bet in all_bets:
            if has_won(bet):
                agency = str(bet.agency)
                if agency not in self.winners_by_agency:
                    self.winners_by_agency[agency] = []
                self.winners_by_agency[agency].append(bet.document)
        
        logging.info(f"Lottery complete. Winners by agency: {self.winners_by_agency}")
        
        # Notify all waiting clients
        for client_sock, agency_id in self.waiting_clients:
            try:
                winners = self.__get_agency_winners(agency_id)
                send_message_to_socket(client_sock, winners)
            except Exception as e:
                logging.error(f"Error notifying waiting client: {e}")
        
        # clean up waiting clients list
        self.waiting_clients = []

    def __handle_winners_query(self, agency_id, client_sock):
        """
        Maneja una consulta de ganadores de una agencia
        Retorna los ganadores si están listos, o None si hay que esperar
        """
        with self.lottery_lock:
            if self.lottery_done:
                # lottery done, return winners immediately
                return self.__get_agency_winners(agency_id)
            else:
                # Lottery not done yet, add to waiting list
                # first check if already in waiting list
                if not any(sock == client_sock for sock, _ in self.waiting_clients):
                    self.waiting_clients.append((client_sock, agency_id))
                    logging.info(f"Agency {agency_id} waiting for lottery results")
                return None  # tell caller to wait

    def __get_agency_winners(self, agency_id):
        """
        Obtiene los DNI ganadores de una agencia específica
        """
        agency_str = str(agency_id)
        if agency_str in self.winners_by_agency:
            winners = self.winners_by_agency[agency_str]
            if winners:
                return ",".join(winners)
        return "NONE"

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