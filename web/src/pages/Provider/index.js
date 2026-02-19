import React from 'react';
import { Container } from 'semantic-ui-react';
import ProvidersTable from '../../components/ProvidersTable';
import Header from '../../components/Header';
import Footer from '../../components/Footer';

const Provider = () => (
    <>
        <Header />
        <Container>
            <ProvidersTable />
        </Container>
        <Footer />
    </>
);

export default Provider;
