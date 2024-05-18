def before_all():
    pass

def before_each(data):
    return {
        "token": "new_token"
    }

def run(data):
    resp = loadsend.http(
        method = "GET",
        url = "https://example.com",
    )

    if resp.status_code == 200:
        resp.success()
    else:
        resp.failed()

def after_each(data):
    pass

def after_all(data):
    pass
