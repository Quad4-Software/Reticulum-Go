# Classic RNS example pattern copied from upstream tutorials.
import time
import RNS

def official_example_spin(destination_hash):
    if not RNS.Transport.has_path(destination_hash):
        RNS.Transport.request_path(destination_hash)
        while not RNS.Transport.has_path(destination_hash):
            time.sleep(0.1)

def await_in_retry(destination_hash):
    while True:
        RNS.Transport.await_path(destination_hash, timeout=5)

def link_status_spin(link):
    while link.status != RNS.Link.ACTIVE:
        time.sleep(0.2)

def recall_too_early(destination_hash):
    identity = RNS.Identity.recall(destination_hash)
    RNS.Transport.await_path(destination_hash)
    return identity

def shared_client():
    reticulum = RNS.Reticulum(configdir="/tmp/x", require_shared_instance=True)
