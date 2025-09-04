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


### Ejercicio N°7:

Modificar los clientes para que notifiquen al servidor al finalizar con el envío de todas las apuestas y así proceder con el sorteo.
Inmediatamente después de la notificacion, los clientes consultarán la lista de ganadores del sorteo correspondientes a su agencia.
Una vez el cliente obtenga los resultados, deberá imprimir por log: `action: consulta_ganadores | result: success | cant_ganadores: ${CANT}`.

El servidor deberá esperar la notificación de las 5 agencias para considerar que se realizó el sorteo e imprimir por log: `action: sorteo | result: success`.
Luego de este evento, podrá verificar cada apuesta con las funciones `load_bets(...)` y `has_won(...)` y retornar los DNI de los ganadores de la agencia en cuestión. Antes del sorteo no se podrán responder consultas por la lista de ganadores con información parcial.

Las funciones `load_bets(...)` y `has_won(...)` son provistas por la cátedra y no podrán ser modificadas por el alumno.

No es correcto realizar un broadcast de todos los ganadores hacia todas las agencias, se espera que se informen los DNIs ganadores que correspondan a cada una de ellas.

## Parte 3: Repaso de Concurrencia
En este ejercicio es importante considerar los mecanismos de sincronización a utilizar para el correcto funcionamiento de la persistencia.

### Ejercicio N°8:

Modificar el servidor para que permita aceptar conexiones y procesar mensajes en paralelo. En caso de que el alumno implemente el servidor en Python utilizando _multithreading_,  deberán tenerse en cuenta las [limitaciones propias del lenguaje](https://wiki.python.org/moin/GlobalInterpreterLock).


## Resolución

### Ejercicio N°7 y N°8:

En este Ejercicio se optó por utilizar la tecnica de miltithreading en python, por el hecho de que sabemos que las operaciones no son I/O intensive, lo que hace que los locks que se utilizan para evitar tener multiples hilos en la sección critica no sean un factor de cuello de botella para el sistema. De la misma manera, los hilos en este caso son mas fáciles de mantener, y como sabemos que no hay una cantidad desorbitante de agencias de lotería no se cree que el overhead de los threads implique un problema para su funcionamiento.

#### Cambios principales en el Cliente:
- Prefijo en mensajes de apuestas: Ahora envía `"BET:" + datos` para distinguir los batches
- Notificación de finalización: Método `notifyFinished()` que envía `"FINISHED"` al servidor
- Consulta de ganadores: Método `queryWinners()` que envía `"WINNERS"` y espera respuesta
- Log requerido: Imprime correctamente `action: consulta_ganadores | result: success | cant_ganadores: X`


#### Cambios principales en el Servidor:
- Threading: Maneja cada cliente en un thread separado para permitir conexiones simultáneas
- Control del sorteo:

    1. Rastrea qué agencias terminaron `agencies_finished`
    3. Ejecuta el sorteo una sola vez


- Sistema de espera: Los clientes que consultan antes del sorteo se agregan a `waiting_clients`
- Distribución de resultados: Cada agencia recibe solo los DNIs de sus propios ganadores
- Log requerido: Imprime `action: sorteo | result: success` cuando se realiza el sorteo


***¿Cómo se logro la correcta implementacion de concurrencia en python teniendo en cuenta [las limitaciones](https://wiki.python.org/moin/GlobalInterpreterLock)?***

- Se usa threading.Lock() para evitar race conditions.
- Los clientes que consultan antes del sorteo quedan "en espera" sin bloquear el servidor.
- Cuando llega el último FINISHED, se ejecuta el sorteo y se notifica a todos los clientes en espera.