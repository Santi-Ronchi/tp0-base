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
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self.running = True
        signal.signal(signal.SIGTERM, self.graceful_shutdown)
        
        # Control de sorteo
        self.lottery_lock = threading.Lock()
        self.agencies_finished = set()  # Agencias que terminaron de enviar
        self.lottery_done = False  # Indica si se realizó el sorteo
        self.winners_by_agency = {}  # DNIs ganadores por agencia
        self.waiting_clients = []  # Clientes esperando resultados
        self.total_agencies = 5  # Total de agencias esperadas

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
                    # Manejar cada cliente en un thread separado
                    client_thread = threading.Thread(
                        target=self.__handle_client_connection,
                        args=(client_sock,)
                    )
                    client_thread.daemon = True
                    client_thread.start()
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
        agency_id = None
        try:
            while True:  # Mantener la conexión abierta para múltiples mensajes
                try:
                    # Read complete message using protocol module
                    msg = read_message_from_socket(client_sock)
                    
                    # Determinar tipo de mensaje
                    if msg.startswith("BET:"):
                        # Es un batch de apuestas
                        batch_data = msg[4:]  # Remover prefijo "BET:"
                        agency_id = self.__process_bets_batch(batch_data)
                        
                        # Send confirmation
                        send_message_to_socket(client_sock, "OK")
                        
                    elif msg == "FINISHED":
                        # Cliente terminó de enviar apuestas
                        if agency_id is not None:
                            self.__handle_finished_notification(agency_id)
                        send_message_to_socket(client_sock, "OK")
                        
                    elif msg == "WINNERS":
                        # Cliente consulta ganadores
                        winners = self.__handle_winners_query(agency_id, client_sock)
                        if winners is not None:
                            # Enviar ganadores inmediatamente si están listos
                            send_message_to_socket(client_sock, winners)
                        # Si winners es None, el cliente será notificado más tarde
                        
                except ConnectionError as e:
                    # Cliente cerró la conexión, es normal
                    logging.debug(f"Client closed connection: {e}")
                    break
                except Exception as e:
                    # Error procesando el mensaje actual
                    logging.error(f"action: message_processing | result: fail | error: {e}")
                    try:
                        send_message_to_socket(client_sock, "ERROR")
                    except:
                        pass
                    break
                    
        finally:
            # Limpiar cliente de la lista de espera si está ahí
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
        
        # Si el batch está vacío, responder OK pero no procesar
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
            
            # Verificar si todas las agencias terminaron
            if len(self.agencies_finished) >= self.total_agencies and not self.lottery_done:
                self.__perform_lottery()

    def __perform_lottery(self):
        """
        Realiza el sorteo y determina los ganadores por agencia
        Debe ser llamado con lottery_lock adquirido
        """
        logging.info("action: sorteo | result: success")
        self.lottery_done = True
        
        # Cargar todas las apuestas
        all_bets = list(load_bets())
        
        # Agrupar ganadores por agencia
        self.winners_by_agency = {}
        for bet in all_bets:
            if has_won(bet):
                agency = str(bet.agency)
                if agency not in self.winners_by_agency:
                    self.winners_by_agency[agency] = []
                self.winners_by_agency[agency].append(bet.document)
        
        logging.info(f"Lottery complete. Winners by agency: {self.winners_by_agency}")
        
        # Notificar a todos los clientes en espera
        for client_sock, agency_id in self.waiting_clients:
            try:
                winners = self.__get_agency_winners(agency_id)
                send_message_to_socket(client_sock, winners)
            except Exception as e:
                logging.error(f"Error notifying waiting client: {e}")
        
        # Limpiar lista de clientes en espera
        self.waiting_clients = []

    def __handle_winners_query(self, agency_id, client_sock):
        """
        Maneja una consulta de ganadores de una agencia
        Retorna los ganadores si están listos, o None si hay que esperar
        """
        with self.lottery_lock:
            if self.lottery_done:
                # Sorteo ya realizado, enviar ganadores inmediatamente
                return self.__get_agency_winners(agency_id)
            else:
                # Sorteo no realizado, agregar cliente a lista de espera
                self.waiting_clients.append((client_sock, agency_id))
                logging.info(f"Agency {agency_id} waiting for lottery results")
                return None  # Indicar que debe esperar

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