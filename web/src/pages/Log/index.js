import React from 'react';
import { Container } from 'semantic-ui-react';
import LogsTable from '../../components/LogsTable';
import Header from '../../components/Header';
import Footer from '../../components/Footer';

const Log = () => (
    <>
        <Header />
        <Container>
            <LogsTable selfOnly={false} />
        </Container>
        <Footer />
    </>
);

export default Log;
