"""
Protocol module for bet communication
Handles serialization/deserialization without using prohibited libraries
ASSUMES: All batches are already pre-sized to fit within 8KB limit
"""

# Protocol constants
FIELD_SEPARATOR = '|'
MESSAGE_SEPARATOR = '\n'
BATCH_SEPARATOR = ';'
MAX_PACKET_SIZE = 8192  # 8KB máximo por paquete
HEADER_SIZE = 4  # 4 bytes para el length prefix
MAX_PAYLOAD_SIZE = MAX_PACKET_SIZE - HEADER_SIZE  # 8188 bytes para datos

def serialize_bet(agency, first_name, last_name, document, birthdate, number):
    """
    Serialize bet data to wire format
    Format: agency|firstName|lastName|document|birthdate|number
    """
    return f"{agency}{FIELD_SEPARATOR}{first_name}{FIELD_SEPARATOR}{last_name}{FIELD_SEPARATOR}{document}{FIELD_SEPARATOR}{birthdate}{FIELD_SEPARATOR}{number}"

def deserialize_bet(message):
    """
    Deserialize wire format to bet data dictionary
    Returns a dictionary with bet fields
    """
    parts = message.split(FIELD_SEPARATOR)
    
    if len(parts) != 6:
        raise ValueError(f"Invalid message format: expected 6 fields, got {len(parts)}")
    
    return {
        "agency": parts[0],
        "first_name": parts[1],
        "last_name": parts[2],
        "document": parts[3],
        "birthdate": parts[4],
        "number": parts[5]
    }

def deserialize_batch(message):
    """
    Deserialize a batch of bets from wire format
    Format: bet1;bet2;bet3...
    Returns a list of dictionaries with bet fields
    ASSUMES: Message is already validated to be within size limits
    """
    if not message:
        return []
    
    # Split by batch separator
    bets_str = message.split(BATCH_SEPARATOR)
    bets = []
    
    for bet_str in bets_str:
        if bet_str.strip():  # Skip empty strings
            bet_data = deserialize_bet(bet_str)
            bets.append(bet_data)
    
    return bets

def serialize_batch(bets):
    """
    Serialize a batch of bets to wire format
    Input: List of bet dictionaries
    Output: String with format bet1;bet2;bet3...
    ASSUMES: Batch size is already validated
    """
    if not bets:
        return ""
    
    serialized_bets = []
    
    for bet in bets:
        serialized = serialize_bet(
            bet['agency'],
            bet['first_name'],
            bet['last_name'],
            bet['document'],
            bet['birthdate'],
            bet['number']
        )
        serialized_bets.append(serialized)
    
    return BATCH_SEPARATOR.join(serialized_bets)

def int_to_bytes(value):
    """
    Convert a 32-bit unsigned integer to 4 bytes (big-endian)
    without using struct library
    """
    if value < 0 or value > 0xFFFFFFFF:
        raise ValueError("Value must be a 32-bit unsigned integer")
    
    # Big-endian: most significant byte first
    return bytes([
        (value >> 24) & 0xFF,
        (value >> 16) & 0xFF,
        (value >> 8) & 0xFF,
        value & 0xFF
    ])

def bytes_to_int(byte_data):
    """
    Convert 4 bytes to integer (big-endian)
    without using struct library
    """
    if len(byte_data) != 4:
        raise ValueError("Expected 4 bytes for integer conversion")
    
    # Big-endian: most significant byte first
    result = 0
    result = (byte_data[0] << 24) | (byte_data[1] << 16) | (byte_data[2] << 8) | byte_data[3]
    return result

def create_message(data):
    """
    Create a length-prefixed message
    Format: [4 bytes length][message data]
    ASSUMES: Data is already validated to fit within limits
    """
    if isinstance(data, str):
        data = data.encode('utf-8')
    
    length_prefix = int_to_bytes(len(data))
    return length_prefix + data

def read_message_from_socket(sock):
    """
    Read a complete length-prefixed message from a socket
    First reads 4 bytes for length, then reads the message body
    ASSUMES: Messages are pre-validated by sender to be within limits
    """
    # Read 4-byte length prefix
    length_data = b""
    while len(length_data) < 4:
        chunk = sock.recv(4 - len(length_data))
        if not chunk:
            raise ConnectionError("Connection closed while reading length")
        length_data += chunk
    
    # Convert bytes to integer
    msg_length = bytes_to_int(length_data)
    
    # Si el mensaje tiene longitud 0, retornar string vacío
    if msg_length == 0:
        return ""
    
    # Read the message body
    msg_data = b""
    while len(msg_data) < msg_length:
        remaining = msg_length - len(msg_data)
        # Optimized chunk size for better performance
        chunk_size = min(remaining, MAX_PACKET_SIZE)
        chunk = sock.recv(chunk_size)
        if not chunk:
            raise ConnectionError("Connection closed while reading message")
        msg_data += chunk
    
    return msg_data.decode('utf-8').strip()

def send_message_to_socket(sock, message):
    """
    Send a complete length-prefixed message to a socket
    Handles partial sends (short writes)
    ASSUMES: Message is already validated to fit within limits
    """
    if isinstance(message, str):
        full_msg = create_message(message)
    else:
        full_msg = message
    
    total_sent = 0
    while total_sent < len(full_msg):
        sent = sock.send(full_msg[total_sent:])
        if sent == 0:
            raise RuntimeError("Socket connection broken")
        total_sent += sent
    
    return total_sent