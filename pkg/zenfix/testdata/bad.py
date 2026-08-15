import time

LINK_TIMEOUT = 15

def wait_link():
    link_ready = False
    while not link_ready:
        time.sleep(0.5)

def wait_path(dest):
    while not Transport.has_path(dest):
        time.sleep(0.05)

class Transport:
    @staticmethod
    def has_path(dest):
        return False

    @staticmethod
    def request_path(dest):
        pass
