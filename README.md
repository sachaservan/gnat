GNAT - Decentralized NAT Traversal
===================

About
----------
GNAT makes it possible to achieve peer-to-peer communication even when both peers are behind a firewall or NAT.  

            ||============[DATA TRAVEL PATH]===========|| 
            ||                                         ||
    ------------------                         -----------------
    |  GNAT NODE #1  |  <=== GNAT NETWORK===>  |  GNAT NODE #2 |
    ------------------                         -----------------
            ||                                          ||
      [HTTP/WebSocket]                           [HTTP/WebSocket]
            ||                                          ||
           -----                                       -----
          |Alice|                                     | Bob |
           -----                                       ----- 


Using the GNAT API:
----------


Setting up a GNAT node:
----------
