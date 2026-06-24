from knowledger.adapters.cli.main import create_app


def main():
    app = create_app()
    app()


if __name__ == "__main__":
    main()
