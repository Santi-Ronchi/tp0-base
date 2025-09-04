# TP0: Docker + Comunicaciones + Concurrencia

En el presente repositorio se provee un esqueleto básico de cliente/servidor, en donde todas las dependencias del mismo se encuentran encapsuladas en containers. Los alumnos deberán resolver una guía de ejercicios incrementales, teniendo en cuenta las condiciones de entrega descritas al final de este enunciado.

 El cliente (Golang) y el servidor (Python) fueron desarrollados en diferentes lenguajes simplemente para mostrar cómo dos lenguajes de programación pueden convivir en el mismo proyecto con la ayuda de containers, en este caso utilizando [Docker Compose](https://docs.docker.com/compose/).

## Instrucciones de uso
El repositorio cuenta con un **Makefile** que incluye distintos comandos en forma de targets. Los targets se ejecutan mediante la invocación de:  **make \<target\>**. Los target imprescindibles para iniciar y detener el sistema son **docker-compose-up** y **docker-compose-down**, siendo los restantes targets de utilidad para el proceso de depuración.

Los targets disponibles son:

| target  | accion  |
|---|---|
|  `docker-compose-up`  | Inicializa el ambiente de desarrollo. Construye las imágenes del cliente y el servidor, inicializa los recursos a utilizar (volúmenes, redes, etc) e inicia los propios containers. |
| `docker-compose-down`  | Ejecuta `docker-compose stop` para detener los containers asociados al compose y luego  `docker-compose down` para destruir todos los recursos asociados al proyecto que fueron inicializados. Se recomienda ejecutar este comando al finalizar cada ejecución para evitar que el disco de la máquina host se llene de versiones de desarrollo y recursos sin liberar. |
|  `docker-compose-logs` | Permite ver los logs actuales del proyecto. Acompañar con `grep` para lograr ver mensajes de una aplicación específica dentro del compose. |
| `docker-image`  | Construye las imágenes a ser utilizadas tanto en el servidor como en el cliente. Este target es utilizado por **docker-compose-up**, por lo cual se lo puede utilizar para probar nuevos cambios en las imágenes antes de arrancar el proyecto. |
| `build` | Compila la aplicación cliente para ejecución en el _host_ en lugar de en Docker. De este modo la compilación es mucho más veloz, pero requiere contar con todo el entorno de Golang y Python instalados en la máquina _host_. |


## Parte 2: Repaso de Comunicaciones

Las secciones de repaso del trabajo práctico plantean un caso de uso denominado **Lotería Nacional**. Para la resolución de las mismas deberá utilizarse como base el código fuente provisto en la primera parte, con las modificaciones agregadas en el ejercicio 4.

### Ejercicio N°5:
Modificar la lógica de negocio tanto de los clientes como del servidor para nuestro nuevo caso de uso.

#### Cliente
Emulará a una _agencia de quiniela_ que participa del proyecto. Existen 5 agencias. Deberán recibir como variables de entorno los campos que representan la apuesta de una persona: nombre, apellido, DNI, nacimiento, numero apostado (en adelante 'número'). Ej.: `NOMBRE=Santiago Lionel`, `APELLIDO=Lorca`, `DOCUMENTO=30904465`, `NACIMIENTO=1999-03-17` y `NUMERO=7574` respectivamente.

Los campos deben enviarse al servidor para dejar registro de la apuesta. Al recibir la confirmación del servidor se debe imprimir por log: `action: apuesta_enviada | result: success | dni: ${DNI} | numero: ${NUMERO}`.



#### Servidor
Emulará a la _central de Lotería Nacional_. Deberá recibir los campos de la cada apuesta desde los clientes y almacenar la información mediante la función `store_bet(...)` para control futuro de ganadores. La función `store_bet(...)` es provista por la cátedra y no podrá ser modificada por el alumno.
Al persistir se debe imprimir por log: `action: apuesta_almacenada | result: success | dni: ${DNI} | numero: ${NUMERO}`.

#### Comunicación:
Se deberá implementar un módulo de comunicación entre el cliente y el servidor donde se maneje el envío y la recepción de los paquetes, el cual se espera que contemple:
* Definición de un protocolo para el envío de los mensajes.
* Serialización de los datos.
* Correcta separación de responsabilidades entre modelo de dominio y capa de comunicación.
* Correcto empleo de sockets, incluyendo manejo de errores y evitando los fenómenos conocidos como [_short read y short write_](https://cs61.seas.harvard.edu/site/2018/FileDescriptors/).


## Resolución

### Ejercicio N°5:

Para este ejercicio primero se implemento un protocolo de serialización/deserealización del tipo protocolo de longitud prefijada (length-prefixed protocol). Los primeros 4 bytes contienen la longitud del mensaje (big-endian) y los siguientes bytes contienen los datos serializados.

Los datos se envían de la forma [largo]agency|firstName|lastName|document|birthdate|number. se utiliza el caracter | para delimitar campos con un formato de texto plano.

las unicas librerías utilizadas son en Go, `bytes` y `encoding/binary`

ejemplo con los datos AGENCIA=1, NOMBRE=Santiago Lionel, APELLIDO=Lorca, DOCUMENTO=30904465, NACIMIENTO=1999-03-17 y NUMERO=7574

- Cliente crea el string 5|Santiago Lionel|Lorca|30904465|1999-03-17|7574.
- Ese string se convierte en bytes (UTF-8). El mensaje tiene 52 bytes contando los separadores.
`[00 00 00 34][35 7C 53 61 6E 74 69 61 67 6F ... 37 34]`
`^ longitud=52   ^ contenido del mensaje (string en UTF-8)`

- Servidor lee los 4 bytes iniciales y sabe que recibirá 52 bytes más.
- Reconstruye el string `5|Santiago Lionel|Lorca|30904465|1999-03-17|7574`.
- Lo separa con split("|").
- Lo convierte en objeto Bet y lo almacena.
- Server envía un OK de vuelta al cliente.

- Cliente loguea el OK como `action: apuesta_enviada | result: success | dni: 30904465 | numero: 7574`.

En este protocolo en particular al saber los bytes que recibiremos de antemano con el header es más facil evitar el short read o short write ya que podemos decirle al cliente o servidor cuantos bytes deben leer para tener todo el mensaje:

snippet del bucle de server.py para no hacer short read:

```
while len(msg_data) < msg_length:
        remaining = msg_length - len(msg_data)
        chunk = sock.recv(min(remaining, MAX_MESSAGE_SIZE))
```

snippet del bucle de client.go para no hacer short write:

```
for totalSent < len(msg) {
		n, err := c.conn.Write(msg[totalSent:])
		if err != nil {
			return err
		}
		totalSent += n
	}
```