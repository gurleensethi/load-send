def before_all():
    pass

def before_each(data):
    return {
        "token": "new_token"
    }

def run(data):
    loadsend.http(
        method = "GET",
        url = "OK",
    )

def after_each(data):
    pass

def after_all(data):
    pass
